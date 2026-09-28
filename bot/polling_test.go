package bot

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestPollerHandlesEveryUpdateOnceAndConfirms(t *testing.T) {
	f := newFakeTelegram()
	f.add(1, 250, 10)
	cfg := testConfig()
	cfg.allowedUpdates = []string{"message"}
	p := newTestPoller(t, f, cfg, 4, 16)
	var (
		mu      sync.Mutex
		handled = map[int64]int{}
		order   = map[int64][]int64{}
	)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- p.run(ctx, func(_ context.Context, u *models.Update) {
			mu.Lock()
			handled[u.UpdateID]++
			order[u.Message.Chat.ID] = append(order[u.Message.Chat.ID], u.UpdateID)
			mu.Unlock()
		})
	}()
	eventually(t, "250 updates", func() bool { mu.Lock(); defer mu.Unlock(); return len(handled) == 250 })
	eventually(t, "the poll after the last batch", func() bool { return len(f.getCalls()) >= 4 })
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}

	for id := int64(1); id <= 250; id++ {
		if handled[id] != 1 {
			t.Errorf("update %d handled %d times, want once", id, handled[id])
		}
	}
	for chat, ids := range order {
		for i := 1; i < len(ids); i++ {
			if ids[i] < ids[i-1] {
				t.Fatalf("chat %d handled %v", chat, ids)
			}
		}
	}
	calls := f.getCalls()
	var offsets []int64
	for _, c := range calls {
		offsets = append(offsets, c.offset)
	}
	if !slices.IsSorted(offsets) || offsets[0] != 0 {
		t.Errorf("offsets = %v, want them from 0 up, never back", offsets)
	}
	if last := calls[len(calls)-1]; last.offset != 251 || last.limit != 1 || last.timeout != 0 {
		t.Errorf("last call = %+v, want the final confirmation with offset 251, limit 1 and timeout 0", last)
	}
	if first := calls[0]; first.limit != 100 || first.timeout != 2 || fmt.Sprint(first.allowed) != "[message]" {
		t.Errorf("first call = %+v, want limit 100, timeout 2 and the allowed updates", first)
	}
	if d := calls[0].deadline; d < 11*time.Second || d > 12*time.Second {
		t.Errorf("getUpdates deadline in %v, want its timeout plus %v", d, pollMargin)
	}
}

func TestPollerShutdown(t *testing.T) {
	tests := []struct {
		name        string
		hold        func(ctx context.Context, release <-chan struct{}) // the handler of update 1
		releaseAt   time.Duration                                      // after the cancellation
		wantErr     error
		wantConfirm bool
		wantCtxErr  bool // the handler saw its context canceled
	}{
		{
			name: "drains the batch in progress", releaseAt: 30 * time.Millisecond, wantConfirm: true,
			hold: func(_ context.Context, release <-chan struct{}) { <-release },
		},
		{
			name: "gives up at the timeout", releaseAt: 500 * time.Millisecond, wantErr: ErrShutdownTimeout,
			hold: func(_ context.Context, release <-chan struct{}) { <-release },
		},
		{
			name: "cancels handlers at the timeout", releaseAt: time.Hour, wantErr: ErrShutdownTimeout, wantCtxErr: true,
			hold: func(ctx context.Context, release <-chan struct{}) {
				select {
				case <-ctx.Done():
				case <-release:
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTelegram()
			f.add(1, 1, 1)
			cfg := testConfig()
			cfg.shutdownTimeout = 100 * time.Millisecond
			p := newTestPoller(t, f, cfg, 2, 2)
			started, release := make(chan struct{}), make(chan struct{})
			var sawCancel atomic.Bool
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				result <- p.run(ctx, func(hctx context.Context, _ *models.Update) {
					close(started)
					tt.hold(hctx, release)
					sawCancel.Store(hctx.Err() != nil)
				})
			}()
			<-started
			cancel()
			timer := time.AfterFunc(tt.releaseAt, func() { close(release) })
			defer func() {
				if timer.Stop() {
					close(release)
				}
			}()

			start := time.Now()
			err := <-result
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("run() = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && time.Since(start) > 400*time.Millisecond {
				t.Errorf("run() returned after %v, want about the shutdown timeout", time.Since(start))
			}
			confirmed := false
			for _, c := range f.getCalls() {
				confirmed = confirmed || c.offset == 2
			}
			if confirmed != tt.wantConfirm {
				t.Errorf("update 1 confirmed = %v, want %v", confirmed, tt.wantConfirm)
			}
			if tt.wantCtxErr {
				eventually(t, "the handler to return", func() bool { return sawCancel.Load() })
			}
		})
	}
}

// TestPollerConfirmsOnlyWhatWasHandled stops the poller while updates of one chat wait behind a
// running one and handing over the next blocks: no offset may pass an update not handled.
func TestPollerConfirmsOnlyWhatWasHandled(t *testing.T) {
	f := newFakeTelegram()
	f.add(1, 5, 1)
	p := newTestPoller(t, f, testConfig(), 1, 1)
	started, release := make(chan struct{}, 5), make(chan struct{})
	var mu sync.Mutex
	handled := map[int64]bool{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- p.run(ctx, func(_ context.Context, u *models.Update) {
			started <- struct{}{}
			<-release
			mu.Lock()
			handled[u.UpdateID] = true
			mu.Unlock()
		})
	}()
	<-started // update 1 runs, update 2 waits in its chat, and handing over update 3 blocks
	time.Sleep(20 * time.Millisecond)
	cancel()
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	for _, c := range f.getCalls() {
		for id := int64(1); id < c.offset; id++ {
			if !handled[id] {
				t.Errorf("getUpdates with offset %d, but update %d was not handled", c.offset, id)
			}
		}
	}
}

func TestPollerFailures(t *testing.T) {
	tests := []struct {
		name       string
		failures   []string
		wantErr    error
		wantErrors int // retried failures reported to onError
		wantCalls  int // getUpdates calls before run returned or the update arrived
	}{
		{name: "retried", failures: []string{"network", "502 Bad Gateway", "429 Too Many Requests"}, wantErrors: 3, wantCalls: 4},
		{name: "revoked token", failures: []string{"401 Unauthorized"}, wantErr: teleiq.ErrUnauthorized, wantCalls: 1},
		{name: "webhook active", failures: []string{"409 Conflict: can't use getUpdates method while webhook is active"}, wantErr: teleiq.ErrConflict, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTelegram()
			f.failures = tt.failures
			f.add(1, 1, 1)
			cfg := testConfig()
			var reported atomic.Int32
			cfg.onError = func(error) { reported.Add(1) }
			p := newTestPoller(t, f, cfg, 1, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handled := make(chan struct{}, 1)
			result := make(chan error, 1)
			go func() {
				result <- p.run(ctx, func(context.Context, *models.Update) { handled <- struct{}{} })
			}()

			if tt.wantErr != nil {
				if err := <-result; !errors.Is(err, tt.wantErr) {
					t.Fatalf("run() = %v, want %v", err, tt.wantErr)
				}
			} else {
				<-handled
				cancel()
				if err := <-result; err != nil {
					t.Fatalf("run() = %v", err)
				}
			}
			if n := int(reported.Load()); n != tt.wantErrors {
				t.Errorf("%d failures reported, want %d", n, tt.wantErrors)
			}
			if n := len(f.getCalls()); n < tt.wantCalls || (tt.wantErr != nil && n != tt.wantCalls) {
				t.Errorf("getUpdates called %d times, want %d", n, tt.wantCalls)
			}
		})
	}
}

func TestPollerStopsItsGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	f := newFakeTelegram()
	f.add(1, 50, 5)
	p := newTestPoller(t, f, testConfig(), 8, 4)
	var handled atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- p.run(ctx, func(context.Context, *models.Update) { handled.Add(1) }) }()
	eventually(t, "50 updates", func() bool { return handled.Load() == 50 })
	cancel()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	eventually(t, "the goroutines to stop", func() bool { return runtime.NumGoroutine() <= before })
}
