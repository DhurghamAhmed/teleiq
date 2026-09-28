package bot

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

// TestPollerResyncsAfterIdling plays the case the resync exists for: after a long idle period,
// Telegram numbers a new update below the offset the bot still holds.
func TestPollerResyncsAfterIdling(t *testing.T) {
	tests := []struct {
		name          string
		idle          time.Duration
		wantOffset    int64 // of the first request after the idle period
		wantDelivered bool  // of the update numbered below the old offset
	}{
		{name: "after six days without updates", idle: 7 * 24 * time.Hour, wantOffset: 0, wantDelivered: true},
		{name: "before six days", idle: 5 * 24 * time.Hour, wantOffset: 4, wantDelivered: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTelegram()
			f.add(1, 3, 1)
			p := newTestPoller(t, f, testConfig(), 2, 4)
			clock := &fakeClock{t: time.Now()}
			p.now = clock.now
			// Update 2 is delivered once in the first batch, and again if the new update 2 arrives.
			var twos atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				result <- p.run(ctx, func(_ context.Context, u *models.Update) {
					if u.UpdateID == 2 {
						twos.Add(1)
					}
				})
			}()
			waitingWith := func(offset int64) func() bool {
				return func() bool { calls := f.getCalls(); return len(calls) > 0 && calls[len(calls)-1].offset == offset }
			}
			eventually(t, "a request with offset 4", waitingWith(4))

			clock.advance(tt.idle)
			before := len(f.getCalls())
			f.wake()
			eventually(t, "the first request after the idle period", func() bool { return len(f.getCalls()) > before })
			if got := f.getCalls()[before].offset; got != tt.wantOffset {
				t.Errorf("first request after %v sent offset %d, want %d", tt.idle, got, tt.wantOffset)
			}

			f.add(2, 2, 1) // numbered below the old offset
			if tt.wantDelivered {
				eventually(t, "the new update 2", func() bool { return twos.Load() == 2 })
				eventually(t, "the offset rebuilt from update 2", waitingWith(3))
			} else {
				time.Sleep(50 * time.Millisecond)
				if n := twos.Load(); n != 1 {
					t.Errorf("update 2 delivered %d times, want only the first: the old offset confirmed the new one", n)
				}
			}
			cancel()
			if err := <-result; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPollerMeasuresIdlenessWithTheWallClock(t *testing.T) {
	p := &poller{now: time.Now}
	if s := p.wallNow().String(); strings.Contains(s, "m=") {
		t.Errorf("wallNow() = %s, want a time without a monotonic reading", s)
	}
}
