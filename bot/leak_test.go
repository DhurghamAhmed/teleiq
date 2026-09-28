package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// These tests run in synctest bubbles, whose clock is fake. When a test function returns, synctest
// fails it if a goroutine it started is still blocked, even on a timer, so each test also checks
// that a stopped bot leaves no goroutine behind.

func TestRunLeavesNoGoroutines(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(f *fakeTelegram, b *Bot)
		run     time.Duration // how long the bot runs before it is stopped
		stop    time.Duration // how long it may take to stop
		wantErr bool
	}{
		{name: "idle", run: time.Hour},
		{name: "handling updates", run: time.Minute, prepare: func(f *fakeTelegram, b *Bot) {
			b.OnMessage(func(context.Context, *Context) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			f.add(1, 50, 5)
		}},
		{name: "stopped while handling", stop: 500 * time.Millisecond, prepare: func(f *fakeTelegram, b *Bot) {
			b.OnMessage(func(context.Context, *Context) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
			f.add(1, 5, 5)
		}},
		{name: "retrying getUpdates", run: time.Hour, prepare: func(f *fakeTelegram, _ *Bot) {
			f.failures = slices.Repeat([]string{"502 Bad Gateway", "network"}, 100)
		}},
		{name: "getUpdates fails for good", wantErr: true, prepare: func(f *fakeTelegram, _ *Bot) {
			f.failures = []string{"401 Unauthorized"}
		}},
		{name: "invalid token", wantErr: true, prepare: func(f *fakeTelegram, _ *Bot) {
			f.me = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFakeTelegram()
				b, _ := newTestBot(t, f)
				if tt.prepare != nil {
					tt.prepare(f, b)
				}
				stop := start(t, b)
				time.Sleep(tt.run)
				synctest.Wait()
				stopping := time.Now()
				if err := stop(); (err != nil) != tt.wantErr {
					t.Errorf("Run() = %v, want error %v", err, tt.wantErr)
				}
				if d := time.Since(stopping); d > tt.stop {
					t.Errorf("Run() took %v to stop, want at most %v", d, tt.stop)
				}
			})
		})
	}
}

func TestRunCanBeRestarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		b, _ := newTestBot(t, f)
		var handled atomic.Int64
		b.OnMessage(func(context.Context, *Context) error {
			handled.Add(1)
			return nil
		})
		for i := range int64(3) {
			stop := start(t, b)
			f.add(10*i+1, 10*i+10, 3)
			time.Sleep(time.Minute)
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
		}
		if n := handled.Load(); n != 30 {
			t.Errorf("%d updates handled, want 30", n)
		}
	})
}

// TestShutdownTimeoutStopsHandling checks that once the shutdown timeout has passed, the updates
// still queued are left for Telegram to send again, instead of being handled after Run returned.
func TestShutdownTimeoutStopsHandling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		b, _ := newTestBot(t, f)
		var mu sync.Mutex
		var handled []int64
		b.OnMessage(func(ctx context.Context, c *Context) error {
			mu.Lock()
			handled = append(handled, c.Update().UpdateID)
			mu.Unlock()
			<-ctx.Done()
			return ctx.Err()
		})
		b.cfg.errorHandler = func(context.Context, *Context, error) {}
		f.add(1, 3, 1)
		stop := start(t, b)
		synctest.Wait()
		if err := stop(); !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("Run() = %v, want ErrShutdownTimeout", err)
		}
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(handled, []int64{1}) {
			t.Errorf("handled %v, want only the update in progress, [1]", handled)
		}
	})
}

func TestRunWebhookLeavesNoGoroutines(t *testing.T) {
	for _, stuck := range []bool{false, true} {
		t.Run(map[bool]string{false: "handled", true: "shutdown timeout"}[stuck], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b, _ := newTestBot(t, newFakeTelegram())
				b.cfg.errorHandler = func(context.Context, *Context, error) {}
				b.OnMessage(func(ctx context.Context, _ *Context) error {
					if stuck {
						<-ctx.Done()
						return ctx.Err()
					}
					time.Sleep(10 * time.Millisecond)
					return nil
				})
				stop := startWebhook(t, b)
				codes := make(chan int, 6)
				for i := range int64(6) {
					go func() {
						rec := httptest.NewRecorder()
						b.WebhookHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(update(i+1, i%2))))
						codes <- rec.Code
					}()
				}
				synctest.Wait()
				err := stop()
				if stuck != errors.Is(err, ErrShutdownTimeout) {
					t.Errorf("RunWebhook() = %v", err)
				}
				want := map[bool]int{false: http.StatusOK, true: http.StatusServiceUnavailable}[stuck]
				for range 6 {
					if code := <-codes; code != want {
						t.Errorf("a request got %d, want %d", code, want)
					}
				}
			})
		})
	}
}
