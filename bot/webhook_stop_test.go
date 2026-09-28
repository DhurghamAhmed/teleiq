package bot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/DhurghamAhmed/teleiq/models"
)

// TestWebhookAnswerAfterStop checks the answer to a request that looks at its update only once
// RunWebhook has returned, when both the end of the handler and the end of RunWebhook are there to
// see: 200 if the handler returned before RunWebhook gave up on the update, 503 otherwise. Every case
// runs many times, since a wrong answer could be one choice of a select among ready channels.
func TestWebhookAnswerAfterStop(t *testing.T) {
	const stuck = 1 // the handler waits in chat 1 until its context ends, and returns at once in others
	tests := []struct {
		name    string
		chats   []int64 // the chats of the updates queued, in order, on a single worker
		wantRun error
		want    []error // the answer for each update: nil for 200, ErrNotRunning for 503
	}{
		{name: "handled, then stopped", chats: []int64{2, 3}, want: []error{nil, nil}},
		{name: "handled, then the shutdown timeout", chats: []int64{2, stuck}, wantRun: ErrShutdownTimeout, want: []error{nil, ErrNotRunning}},
		{name: "handling and queued at the shutdown timeout", chats: []int64{stuck, 2}, wantRun: ErrShutdownTimeout, want: []error{ErrNotRunning, ErrNotRunning}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b, _ := newTestBot(t, newFakeTelegram(), WithWorkers(1))
				b.OnMessage(func(ctx context.Context, c *Context) error {
					if c.Chat().ID == stuck {
						<-ctx.Done()
						return ctx.Err()
					}
					return nil
				})
				b.cfg.errorHandler = func(context.Context, *Context, error) {}
				for range 100 {
					stop := startWebhook(t, b)
					hook := b.hook.Load()
					var dones []<-chan struct{}
					for i, chat := range tt.chats {
						var u models.Update
						if err := json.Unmarshal([]byte(update(int64(i+1), chat)), &u); err != nil {
							t.Fatal(err)
						}
						j, done := hook.job(&u, false)
						if err := hook.sched.trySubmit(j); err != nil {
							t.Fatal(err)
						}
						dones = append(dones, done)
					}
					synctest.Wait()
					if err := stop(); !errors.Is(err, tt.wantRun) {
						t.Fatalf("RunWebhook() = %v, want %v", err, tt.wantRun)
					}
					synctest.Wait() // the handler given up on returns once its context ends
					for i, done := range dones {
						if err := hook.wait(context.Background(), done); !errors.Is(err, tt.want[i]) {
							t.Fatalf("update %d: wait() = %v, want %v", i+1, err, tt.want[i])
						}
					}
				}
			})
		})
	}
}
