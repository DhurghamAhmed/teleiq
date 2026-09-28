package bot

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func newCommandsBot(t *testing.T, opts ...Option) (*teleiqtest.Server, *Bot) {
	t.Helper()
	srv := teleiqtest.NewServer()
	t.Cleanup(srv.Close)
	c, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return srv, New(c, opts...)
}

// TestWithCommands checks that the bot sets its commands right after getMe, before it handles
// updates, with polling and with a webhook, and only when WithCommands is given.
func TestWithCommands(t *testing.T) {
	commands := []models.BotCommand{{Command: "start", Description: "Say hello"}, {Command: "help", Description: "List the commands"}}
	const sent = `[{"command":"start","description":"Say hello"},{"command":"help","description":"List the commands"}]`
	tests := []struct {
		name     string
		run      func(*Bot, context.Context) error
		commands []models.BotCommand
		want     []string
	}{
		{"Run", (*Bot).Run, commands, []string{"getMe", "setMyCommands"}},
		{"RunWebhook", (*Bot).RunWebhook, commands, []string{"getMe", "setMyCommands"}},
		{"Run without commands", (*Bot).Run, nil, []string{"getMe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			given := slices.Clone(tt.commands)
			if given != nil {
				opts = append(opts, WithCommands(given...))
			}
			srv, b := newCommandsBot(t, opts...)
			if given != nil {
				given[0].Command = "changed" // the bot keeps its own copy
			}
			stop := teleiqtest.Start(t, func(ctx context.Context) error { return tt.run(b, ctx) })
			srv.Wait(t, tt.want[len(tt.want)-1], 1)
			if err := stop(); err != nil {
				t.Fatal(err)
			}

			var methods []string
			for _, r := range srv.Requests("") {
				methods = append(methods, r.Method)
			}
			if !slices.Equal(methods, tt.want) {
				t.Errorf("requests = %v, want %v", methods, tt.want)
			}
			if tt.commands == nil {
				return
			}
			got, err := json.Marshal(srv.Requests("setMyCommands")[0].Params["commands"])
			if err != nil || string(got) != sent {
				t.Errorf("commands = %s, %v; want %s", got, err, sent)
			}
		})
	}
}

// TestWithCommandsFails checks that a bot that cannot set its commands does not start. Each Run
// has a timeout, so that a bot that starts polling by mistake fails the test instead of hanging it.
func TestWithCommandsFails(t *testing.T) {
	t.Run("no commands", func(t *testing.T) {
		srv, b := newCommandsBot(t, WithCommands())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.Run(ctx); err == nil || !strings.Contains(err.Error(), "WithCommands") {
			t.Errorf("Run() = %v, want the misuse of WithCommands", err)
		}
		if n := len(srv.Requests("")); n != 0 {
			t.Errorf("%d requests made, want none", n)
		}
	})
	t.Run("rejected", func(t *testing.T) {
		srv, b := newCommandsBot(t, startCommands)
		srv.Fail("setMyCommands", &teleiq.Error{ErrorCode: 400, Description: "Bad Request: invalid command"})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var apiErr *teleiq.Error
		if err := b.Run(ctx); !errors.As(err, &apiErr) || apiErr.ErrorCode != 400 {
			t.Errorf("Run() = %v, want the error of setMyCommands", err)
		}
		if b.running.Load() {
			t.Error("the bot still counts as running after it failed to start")
		}
	})
}
