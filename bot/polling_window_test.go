package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// runPoller runs p until the returned stop is called; handle records what it needs.
func runPoller(t *testing.T, p *poller, handle func(context.Context, *models.Update)) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- p.run(ctx, handle) }()
	return func() error { cancel(); return <-result }
}

// TestPollerDoesNotWaitForAStuckUpdate: B arrives while A, of another chat, is stuck; B is handled
// within the refetch interval, and A is confirmed only once handled.
func TestPollerDoesNotWaitForAStuckUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		f.add(1, 1, 1) // update 1, chat 1
		p := newTestPoller(t, f, testConfig(), 4, 128)
		release := make(chan struct{})
		var mu sync.Mutex
		handledAt := map[int64]time.Time{}
		start := time.Now()
		stop := runPoller(t, p, func(_ context.Context, u *models.Update) {
			if u.UpdateID == 1 {
				<-release
			}
			mu.Lock()
			handledAt[u.UpdateID] = time.Now()
			mu.Unlock()
		})
		time.Sleep(time.Second)
		f.add(2, 2, 3) // update 2, chat 3
		time.Sleep(time.Second)
		synctest.Wait()
		mu.Lock()
		at, ok := handledAt[2]
		mu.Unlock()
		if !ok || at.Sub(start) > time.Second+refetchInterval {
			t.Errorf("update 2 handled %v after it arrived (ok %v), want within %v", at.Sub(start)-time.Second, ok, refetchInterval)
		}
		for _, c := range f.getCalls() {
			if c.offset > 1 {
				t.Errorf("getUpdates with offset %d while update 1 was in progress", c.offset)
			}
		}
		close(release)
		if err := stop(); err != nil {
			t.Fatal(err)
		}
		if calls := f.getCalls(); calls[len(calls)-1].offset != 3 {
			t.Errorf("last offset %d, want 3", calls[len(calls)-1].offset)
		}
	})
}

// TestPollerWindow: while update 1 is stuck, Telegram returns updates 1 to 100 only, so update
// 101 waits for update 1, and nothing is asked every interval once the window is full.
func TestPollerWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		f.add(1, 101, 1000) // a chat each
		p := newTestPoller(t, f, testConfig(), 8, 128)
		release := make(chan struct{})
		var mu sync.Mutex
		handled := map[int64]bool{}
		stop := runPoller(t, p, func(_ context.Context, u *models.Update) {
			if u.UpdateID == 1 {
				<-release
			}
			mu.Lock()
			handled[u.UpdateID] = true
			mu.Unlock()
		})
		time.Sleep(time.Minute)
		synctest.Wait()
		mu.Lock()
		n, got101 := len(handled), handled[101]
		mu.Unlock()
		if n != 99 || got101 {
			t.Errorf("while update 1 was stuck: %d handled, update 101 handled %v; want 2 to 100 only", n, got101)
		}
		if calls := len(f.getCalls()); calls > 3 {
			t.Errorf("%d getUpdates in a minute with a full window, want no polling until update 1 is handled", calls)
		}
		close(release)
		time.Sleep(time.Second)
		synctest.Wait()
		mu.Lock()
		got101 = handled[101]
		mu.Unlock()
		if !got101 {
			t.Error("update 101 not handled after update 1")
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestPollerRefetchesSlowly: with update 1 stuck and nothing else arriving, the poller asks about
// twice a second.
func TestPollerRefetchesSlowly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		f.add(1, 1, 1)
		p := newTestPoller(t, f, testConfig(), 2, 128)
		release := make(chan struct{})
		stop := runPoller(t, p, func(_ context.Context, u *models.Update) { <-release })
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if n := len(f.getCalls()); n < 15 || n > 25 {
			t.Errorf("%d getUpdates in 10s with update 1 stuck, want about 20", n)
		}
		close(release)
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})
}

// checkOffsets passes requests to f after calling check with the offset of each getUpdates.
type checkOffsets struct {
	f     *fakeTelegram
	check func(offset int64)
}

func (c checkOffsets) Do(ctx context.Context, req *teleiq.Request) (*teleiq.Response, error) {
	if req.Method == "getUpdates" {
		body, _ := io.ReadAll(req.Body)
		var p struct{ Offset int64 }
		_ = json.Unmarshal(body, &p)
		c.check(p.Offset)
		req.Body = bytes.NewReader(body)
	}
	return c.f.Do(ctx, req)
}

// TestPollerModel handles 2000 updates of 30 chats with random delays, a few of them long, while
// updates keep arriving, and checks that every update is handled once, in the order of its chat,
// and that no offset passes an update not handled yet.
func TestPollerModel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const total, chats = 2000, 30
		f := newFakeTelegram()
		var mu sync.Mutex
		handled := map[int64]int{}
		last := map[int64]int64{}
		var bad []string
		client, err := teleiq.NewClient("123:test", teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}),
			teleiq.WithTransport(checkOffsets{f, func(offset int64) {
				mu.Lock()
				defer mu.Unlock()
				// Every update below the offset is confirmed by it, so each must have been handled.
				for id := int64(1); id < offset; id++ {
					if handled[id] == 0 {
						bad = append(bad, "offset passes an update not handled")
						return
					}
				}
			}}))
		if err != nil {
			t.Fatal(err)
		}
		p := newPoller(client, newScheduler(8, 128), testConfig())
		stop := runPoller(t, p, func(_ context.Context, u *models.Update) {
			d := time.Duration(rand.IntN(20)) * time.Millisecond
			if rand.IntN(100) == 0 {
				d = 3 * time.Second
			}
			time.Sleep(d)
			mu.Lock()
			defer mu.Unlock()
			handled[u.UpdateID]++
			chat := u.Message.Chat.ID
			if u.UpdateID <= last[chat] {
				bad = append(bad, "out of order")
			}
			last[chat] = u.UpdateID
		})
		for id := int64(1); id <= total; id += 10 {
			f.add(id, id+9, chats)
			time.Sleep(time.Duration(rand.IntN(30)) * time.Millisecond)
		}
		for {
			time.Sleep(time.Second)
			mu.Lock()
			n := len(handled)
			mu.Unlock()
			if n == total {
				break
			}
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
		for id := int64(1); id <= total; id++ {
			if handled[id] != 1 {
				t.Errorf("update %d handled %d times", id, handled[id])
			}
		}
		if len(bad) > 0 {
			t.Errorf("%d violations, the first: %s", len(bad), bad[0])
		}
		t.Logf("%d getUpdates for %d updates", len(f.getCalls()), total)
	})
}

// TestPollerShutdownTimeoutConfirmsWhatWasHandled: at the shutdown timeout, updates 1 and 3 are
// handled and update 2 is not, so the bot confirms update 1 only; a handler that returns because
// its context was canceled does not count as handled.
func TestPollerShutdownTimeoutConfirmsWhatWasHandled(t *testing.T) {
	tests := []struct {
		name  string
		first time.Duration                                      // how long update 1 takes
		hold  func(ctx context.Context, release <-chan struct{}) // the handler of update 2
	}{
		{"handler ignores its context", 0, func(_ context.Context, release <-chan struct{}) { <-release }},
		{"handler returns once canceled", 0, func(ctx context.Context, _ <-chan struct{}) { <-ctx.Done() }},
		// Update 1 is handled after the last getUpdates, so only the final one confirms it.
		{"update 1 handled while draining", 1500 * time.Millisecond, func(_ context.Context, release <-chan struct{}) { <-release }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFakeTelegram()
				f.add(1, 3, 1000) // a chat each
				cfg := testConfig()
				cfg.shutdownTimeout = time.Second
				p := newTestPoller(t, f, cfg, 4, 128)
				release := make(chan struct{})
				defer close(release)
				stop := runPoller(t, p, func(ctx context.Context, u *models.Update) {
					switch u.UpdateID {
					case 1:
						time.Sleep(tt.first)
					case 2:
						tt.hold(ctx, release)
					}
				})
				time.Sleep(time.Second)
				synctest.Wait()
				if err := stop(); !errors.Is(err, ErrShutdownTimeout) {
					t.Fatalf("run() = %v, want ErrShutdownTimeout", err)
				}
				calls := f.getCalls()
				if last := calls[len(calls)-1]; last.offset != 2 {
					t.Errorf("last offset %d, want 2: update 1 confirmed, update 2 delivered again", last.offset)
				}
			})
		})
	}
}
