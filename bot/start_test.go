package bot

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// slowGetMe answers getMe only when its request ends, like a Bot API that is slow to answer.
var slowGetMe = teleiq.TransportFunc(func(ctx context.Context, _ *teleiq.Request) (*teleiq.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
})

// slowCommands answers getMe at once and setMyCommands only when its request ends.
var slowCommands = teleiq.TransportFunc(func(ctx context.Context, r *teleiq.Request) (*teleiq.Response, error) {
	if r.Method == "getMe" {
		return &teleiq.Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Bot","username":"test_bot"}}`)}, nil
	}
	return slowGetMe(ctx, r)
})

var startCommands = WithCommands(models.BotCommand{Command: "start", Description: "Say hello"})

func newStartingBot(t *testing.T, tr teleiq.Transport, opts ...Option) *Bot {
	t.Helper()
	c, err := teleiq.NewClient(testToken, teleiq.WithTransport(tr), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return New(c, opts...)
}

// TestStoppedWhileStarting checks that a bot stopped during its initial getMe, or while it sets
// its commands, stops cleanly, as it does once it runs, and that a start that fails for another
// reason is still an error.
func TestStoppedWhileStarting(t *testing.T) {
	runs := map[string]func(*Bot, context.Context) error{"Run": (*Bot).Run, "RunWebhook": (*Bot).RunWebhook}
	starts := []struct {
		name string
		tr   teleiq.Transport
		opts []Option
	}{
		{"getMe", slowGetMe, nil},
		{"setMyCommands", slowCommands, []Option{startCommands}},
	}
	for name, run := range runs {
		for _, s := range starts {
			t.Run(name+" during "+s.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					b := newStartingBot(t, s.tr, s.opts...)
					ctx, cancel := context.WithCancel(context.Background())
					result := make(chan error, 1)
					go func() { result <- run(b, ctx) }()
					time.Sleep(time.Second)
					cancel()
					if err := <-result; err != nil {
						t.Errorf("%s() stopped while starting = %v, want nil", name, err)
					}

					ended, end := context.WithCancel(context.Background())
					end()
					if err := run(b, ended); err != nil {
						t.Errorf("%s() with an ended context = %v, want nil", name, err)
					}
				})
			})
		}
	}

	for _, s := range starts {
		t.Run(s.name+" times out", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// The context of the caller is still alive: the default timeout of the client ended the call.
				if err := newStartingBot(t, s.tr, s.opts...).Run(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("Run() = %v, want the timeout of %s", err, s.name)
				}
			})
		})
	}
	t.Run("token rejected", func(t *testing.T) {
		rejected := teleiq.TransportFunc(func(context.Context, *teleiq.Request) (*teleiq.Response, error) {
			return &teleiq.Response{StatusCode: 401, Body: []byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`)}, nil
		})
		if err := newStartingBot(t, rejected).Run(context.Background()); !errors.Is(err, teleiq.ErrUnauthorized) {
			t.Errorf("Run() = %v, want ErrUnauthorized", err)
		}
	})
}
