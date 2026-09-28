package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq"
)

// weeksTelegram serves getUpdates like the Bot API for weeks of fake time: bursts of updates for
// many chats, long polls that end empty, failed requests, and a quiet week. It keeps no history, so
// what grows during a run is the bot's.
type weeksTelegram struct {
	mu            sync.Mutex
	rng           *rand.Rand
	quietFrom     time.Time
	quietUntil    time.Time
	next          int64    // the update_id of the next new update
	unconfirmed   []string // updates sent but not yet confirmed by an offset
	firstID       int64    // the update_id of unconfirmed[0]
	withoutOffset int      // requests without an offset after the first update
}

func (w *weeksTelegram) Do(ctx context.Context, req *teleiq.Request) (*teleiq.Response, error) {
	if req.Method == "getMe" {
		return reply(`{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}`), nil
	}
	var p struct{ Offset, Timeout int64 }
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &p)

	w.mu.Lock()
	if p.Offset == 0 && w.next > 1 {
		w.withoutOffset++
	}
	for p.Offset > w.firstID && len(w.unconfirmed) > 0 {
		w.unconfirmed, w.firstID = w.unconfirmed[1:], w.firstID+1
	}
	now := time.Now()
	quiet := !now.Before(w.quietFrom) && now.Before(w.quietUntil)
	switch n := w.rng.IntN(100); {
	case n < 2:
		w.mu.Unlock()
		return nil, errors.New("connection reset by peer")
	case n < 4:
		w.mu.Unlock()
		return &teleiq.Response{StatusCode: 502, Body: []byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`)}, nil
	case n < 9 && len(w.unconfirmed) == 0 && !quiet:
		w.firstID = w.next
		for range 1 + w.rng.IntN(20) {
			w.unconfirmed = append(w.unconfirmed, fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1,"chat":{"id":%d,"type":"private"},"text":"hi"}}`,
				w.next, w.next, 1+w.rng.IntN(1000)))
			w.next++
		}
	}
	batch := strings.Join(w.unconfirmed, ",")
	w.mu.Unlock()
	if batch == "" {
		select {
		case <-time.After(time.Duration(p.Timeout) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return reply("[" + batch + "]"), nil
}

func reply(result string) *teleiq.Response {
	return &teleiq.Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":` + result + `}`)}
}

// TestLongRun runs a bot for three weeks of fake time and checks that it handles every update once
// and in order, gets past a quiet week, and ends the run with as many goroutines and about as much
// memory as it had after the first day.
func TestLongRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		day := 24 * time.Hour
		w := &weeksTelegram{rng: rand.New(rand.NewPCG(1, 2)), next: 1, firstID: 1,
			quietFrom: time.Now().Add(7 * day), quietUntil: time.Now().Add(14 * day)}
		client, err := teleiq.NewClient(testToken, teleiq.WithTransport(w))
		if err != nil {
			t.Fatal(err)
		}
		var failures int
		b := New(client, WithErrorHandler(func(context.Context, *Context, error) {
			failures++
		}))
		var handled int64
		last := map[int64]int64{} // chat -> the last update_id handled in it
		var mu sync.Mutex
		b.OnMessage(func(_ context.Context, c *Context) error {
			mu.Lock()
			defer mu.Unlock()
			id := c.Update().UpdateID
			if id <= last[c.Chat().ID] {
				t.Errorf("update %d handled after update %d of the same chat", id, last[c.Chat().ID])
			}
			last[c.Chat().ID] = id
			handled++
			return nil
		})
		stop := start(t, b)

		measure := func() (goroutines int, heap uint64) {
			synctest.Wait()
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return bubbleGoroutines(t), m.HeapAlloc
		}
		time.Sleep(day)
		goroutines, heap := measure()
		time.Sleep(20 * day)
		goroutinesAfter, heapAfter := measure()
		if err := stop(); err != nil {
			t.Fatalf("Run() = %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if want := w.next - 1; handled != want || want < 20000 {
			t.Errorf("handled %d updates, want each of the %d sent once", handled, want)
		}
		if w.withoutOffset == 0 {
			t.Error("the quiet week did not make the bot stop sending its offset")
		}
		if failures == 0 {
			t.Error("no failed getUpdates was reported")
		}
		if goroutinesAfter != goroutines {
			t.Errorf("%d goroutines after three weeks, %d after the first day", goroutinesAfter, goroutines)
		}
		if heapAfter > heap+256<<10 {
			t.Errorf("heap grew from %d to %d bytes", heap, heapAfter)
		}
		t.Logf("%d updates, %d failures, heap %d -> %d bytes", handled, failures, heap, heapAfter)
	})
}

// bubbleGoroutines counts the goroutines of synctest bubbles, here the one of the calling test.
// runtime.NumGoroutine also counts those that earlier tests left to end on their own, such as HTTP
// dials, which end at any moment.
func bubbleGoroutines(t *testing.T) int {
	t.Helper()
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	count := 0
	for line := range strings.Lines(string(buf)) {
		if strings.HasPrefix(line, "goroutine ") && strings.Contains(line, ", synctest bubble ") {
			count++
		}
	}
	if count == 0 {
		t.Fatal("no goroutine of a synctest bubble in the stacks of the process")
	}
	return count
}
