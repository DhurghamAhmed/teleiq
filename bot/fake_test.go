package bot

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
)

type getUpdatesCall struct {
	offset, limit, timeout int64
	deadline               time.Duration // left on the context of the request
	allowed                []string
}

// fakeTelegram serves getUpdates like the Bot API: an offset confirms, and so deletes, every update
// below it, even one that arrives later with a lower number; a request without an offset returns
// the oldest unconfirmed updates. A request without updates waits until new ones arrive, wake is
// called or its context ends.
type fakeTelegram struct {
	mu       sync.Mutex
	updates  map[int64]string // unconfirmed update_id -> the update as JSON
	calls    []getUpdatesCall // every getUpdates, in order
	failures []string         // replies for the first calls, then real batches
	me       string           // the reply to getMe, if not the default
	getMes   int
	arrived  chan struct{}
}

func newFakeTelegram() *fakeTelegram {
	return &fakeTelegram{updates: map[int64]string{}, arrived: make(chan struct{}, 1)}
}

// add makes messages with the update_ids first..last available, spread over chats.
func (f *fakeTelegram) add(first, last int64, chats int64) {
	f.mu.Lock()
	for id := first; id <= last; id++ {
		f.updates[id] = fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1,"chat":{"id":%d,"type":"private"}}}`, id, id, id%chats+1)
	}
	f.mu.Unlock()
	f.wake()
}

// addJSON makes an update with the given fields available, such as `"inline_query":{...}`.
func (f *fakeTelegram) addJSON(id int64, fields string) {
	f.mu.Lock()
	f.updates[id] = fmt.Sprintf(`{"update_id":%d,%s}`, id, fields)
	f.mu.Unlock()
	f.wake()
}

// wake ends a request that is waiting for updates.
func (f *fakeTelegram) wake() {
	select {
	case f.arrived <- struct{}{}:
	default:
	}
}

func (f *fakeTelegram) Do(ctx context.Context, req *teleiq.Request) (*teleiq.Response, error) {
	if req.Method == "getMe" {
		f.mu.Lock()
		f.getMes++
		reply := cmp.Or(f.me, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
		f.mu.Unlock()
		return &teleiq.Response{StatusCode: 200, Body: []byte(reply)}, nil
	}
	var p struct {
		Offset, Limit, Timeout int64
		AllowedUpdates         []string `json:"allowed_updates"`
	}
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &p)
	deadline, _ := ctx.Deadline()
	f.mu.Lock()
	f.calls = append(f.calls, getUpdatesCall{p.Offset, p.Limit, p.Timeout, time.Until(deadline), p.AllowedUpdates})
	if len(f.failures) > 0 {
		reply := f.failures[0]
		f.failures = f.failures[1:]
		f.mu.Unlock()
		if reply == "network" {
			return nil, errors.New("connection reset by peer")
		}
		code := strings.Fields(reply)[0]
		return &teleiq.Response{StatusCode: 500, Body: []byte(fmt.Sprintf(`{"ok":false,"error_code":%s,"description":%q}`, code, reply))}, nil
	}
	f.mu.Unlock()
	// A signal sent before this request is stale; updates added before it are found by the check.
	select {
	case <-f.arrived:
	default:
	}
	for woke := false; ; woke = true {
		f.mu.Lock()
		if p.Offset > 0 {
			for id := range f.updates {
				if id < p.Offset {
					delete(f.updates, id)
				}
			}
		}
		ids := slices.Sorted(maps.Keys(f.updates))
		var batch []string
		for _, id := range ids[:min(len(ids), int(max(p.Limit, 1)))] {
			batch = append(batch, f.updates[id])
		}
		f.mu.Unlock()
		if len(batch) > 0 || p.Timeout == 0 || woke {
			return &teleiq.Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":[` + strings.Join(batch, ",") + `]}`)}, nil
		}
		select {
		case <-f.arrived:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (f *fakeTelegram) meCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getMes
}

func (f *fakeTelegram) getCalls() []getUpdatesCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]getUpdatesCall(nil), f.calls...)
}

func testConfig() pollConfig {
	return pollConfig{timeout: 2 * time.Second, limit: 100, shutdownTimeout: time.Second, ackTimeout: time.Second,
		retryBase: time.Millisecond, retryMax: 2 * time.Millisecond}
}

func newTestPoller(t *testing.T, f *fakeTelegram, cfg pollConfig, workers, queue int) *poller {
	t.Helper()
	c, err := teleiq.NewClient("123:test", teleiq.WithTransport(f), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return newPoller(c, newScheduler(workers, queue), cfg)
}

// eventually waits up to two seconds for cond.
func eventually(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
