package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
)

const testToken = "123456789:Ab1_-Ab1_-Ab1_-Ab1_-Ab1_-Ab1_-Ab1_-"

func newTestBot(t testing.TB, f *fakeTelegram, opts ...Option) (*Bot, *teleiq.Client) {
	t.Helper()
	c, err := teleiq.NewClient(testToken, teleiq.WithTransport(f), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return New(c, append([]Option{WithPollTimeout(2 * time.Second), WithShutdownTimeout(time.Second)}, opts...)...), c
}

// start runs b and returns a function that stops it and returns the result of Run.
func start(t *testing.T, b *Bot) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- b.Run(ctx) }()
	return func() error {
		cancel()
		return <-result
	}
}

// handledUpTo waits until Telegram was asked for the updates after id, so every update up to id
// has been handled.
func handledUpTo(t *testing.T, f *fakeTelegram, id int64) {
	t.Helper()
	eventually(t, fmt.Sprintf("the updates up to %d", id), func() bool {
		calls := f.getCalls()
		return len(calls) > 0 && calls[len(calls)-1].offset > id
	})
}

func message(id int64, text string) string {
	return fmt.Sprintf(`"message":{"message_id":%d,"date":1,"chat":{"id":%d,"type":"private"},"text":%q}`, id, id, text)
}

func TestBotRoutes(t *testing.T) {
	f := newFakeTelegram()
	updates := []struct {
		fields string
		want   string // the handler that should run, or "" for none
	}{
		{message(1, "/start"), "start"},
		{message(2, "/start@test_bot hello"), "start"},
		{message(3, "/START"), "start"},
		{message(4, "/start@Test_Bot"), "start"},
		{message(5, "/start@other_bot"), "message"},
		{message(6, "/starter"), "message"},
		{message(7, "hello /start"), "message"},
		{message(8, "ping"), "ping"},
		{message(9, "/help"), "help"},
		{`"callback_query":{"id":"q","from":{"id":1,"is_bot":false,"first_name":"A"},"chat_instance":"c"}`, "callback"},
		{`"edited_message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"/start"}`, ""},
		{`"inline_query":{"id":"i","from":{"id":1,"is_bot":false,"first_name":"A"},"query":"","offset":""}`, ""},
		{`"message":{"message_id":13,"date":1,"chat":{"id":13,"type":"private"}}`, "message"},
	}
	for i, u := range updates {
		f.addJSON(int64(i+1), u.fields)
	}
	b, _ := newTestBot(t, f)
	var mu sync.Mutex
	ran := map[int64]string{}
	record := func(name string) Handler {
		return func(_ context.Context, c *Context) error {
			mu.Lock()
			ran[c.Update().UpdateID] = name
			mu.Unlock()
			return nil
		}
	}
	b.OnCommand("start", record("start"))
	b.OnCommand("/help", record("help"))
	b.Handle(FilterFunc(func(c *Context) bool {
		m := c.Update().Message
		return m != nil && m.Text != nil && *m.Text == "ping"
	}), record("ping"))
	b.OnMessage(record("message"))
	b.OnCallback(record("callback"))

	stop := start(t, b)
	handledUpTo(t, f, int64(len(updates)))
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for i, u := range updates {
		if got := ran[int64(i+1)]; got != u.want {
			t.Errorf("update %d (%s) ran %q, want %q", i+1, u.fields[:min(len(u.fields), 60)], got, u.want)
		}
	}
}

func TestBotContext(t *testing.T) {
	f := newFakeTelegram()
	f.add(1, 1, 1)
	b, client := newTestBot(t, f)
	got := make(chan *Context, 1)
	b.OnMessage(func(_ context.Context, c *Context) error { got <- c; return nil })
	stop := start(t, b)
	c := <-got
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if c.Update().UpdateID != 1 || c.Client() != client {
		t.Errorf("Context has update %d and client %p, want 1 and %p", c.Update().UpdateID, c.Client(), client)
	}
}

type reportedError struct {
	update int64 // 0 when the Context is nil
	err    string
}

func TestBotErrorHandler(t *testing.T) {
	f := newFakeTelegram()
	f.failures = []string{"network"}
	f.add(1, 2, 1)
	var mu sync.Mutex
	var reports []reportedError
	b, _ := newTestBot(t, f, WithErrorHandler(func(_ context.Context, c *Context, err error) {
		r := reportedError{err: err.Error()}
		if c != nil {
			r.update = c.Update().UpdateID
		}
		mu.Lock()
		reports = append(reports, r)
		mu.Unlock()
	}))
	b.OnMessage(func(_ context.Context, c *Context) error {
		if c.Update().UpdateID == 1 {
			return errors.New("cannot handle 1")
		}
		return nil
	})

	stop := start(t, b)
	handledUpTo(t, f, 2)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	want := []reportedError{
		{0, "teleiq: getUpdates: connection reset by peer"},
		{1, "cannot handle 1"},
	}
	if fmt.Sprint(reports) != fmt.Sprint(want) {
		t.Errorf("reported %v, want %v", reports, want)
	}
}

func TestBotDefaultErrorHandler(t *testing.T) {
	var buf bytes.Buffer
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	f := newFakeTelegram()
	f.failures = []string{"network"}
	f.add(1, 1, 1)
	b, _ := newTestBot(t, f)
	b.OnMessage(func(context.Context, *Context) error {
		return errors.New("request to /bot" + testToken + "/sendMessage failed")
	})
	stop := start(t, b)
	handledUpTo(t, f, 1)
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	logs := buf.String()
	for _, want := range []string{
		`level=WARN msg="bot: receiving updates failed" error="teleiq: getUpdates: connection reset by peer"`,
		`level=ERROR msg="bot: handler failed" update_id=1 error="request to /bot<redacted>/sendMessage failed"`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs do not contain %s:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, testToken) {
		t.Errorf("logs contain the token:\n%s", logs)
	}
}

func TestBotMisuse(t *testing.T) {
	noop := func(context.Context, *Context) error { return nil }
	tests := []struct {
		name      string
		nilClient bool
		opts      []Option
		setup     func(b *Bot)
		ctx       context.Context
		wantInErr string
	}{
		{name: "nil client", nilClient: true, wantInErr: "nil client"},
		{name: "no workers", opts: []Option{WithWorkers(0)}, wantInErr: "WithWorkers"},
		{name: "no queue", opts: []Option{WithQueueSize(0)}, wantInErr: "WithQueueSize"},
		{name: "nil error handler", opts: []Option{WithErrorHandler(nil)}, wantInErr: "WithErrorHandler"},
		{name: "no shutdown timeout", opts: []Option{WithShutdownTimeout(0)}, wantInErr: "WithShutdownTimeout"},
		{name: "negative poll timeout", opts: []Option{WithPollTimeout(-time.Second)}, wantInErr: "WithPollTimeout"},
		{name: "nil filter", setup: func(b *Bot) { b.Handle(nil, noop) }, wantInErr: "nil filter"},
		{name: "nil handler", setup: func(b *Bot) { b.OnMessage(nil) }, wantInErr: "nil handler"},
		{name: "invalid command", setup: func(b *Bot) { b.OnCommand("say hi", noop) }, wantInErr: "OnCommand"},
		{name: "empty command", setup: func(b *Bot) { b.OnCommand("/", noop) }, wantInErr: "OnCommand"},
		{name: "first misuse wins", opts: []Option{WithWorkers(0), WithQueueSize(0)}, setup: func(b *Bot) { b.Handle(nil, noop) }, wantInErr: "WithWorkers"},
		{name: "nil context", ctx: nil, wantInErr: "nil Context"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeTelegram()
			b, _ := newTestBot(t, f, tt.opts...)
			if tt.nilClient {
				b = New(nil)
			}
			if tt.setup != nil {
				tt.setup(b)
			}
			ctx := tt.ctx
			if ctx == nil && tt.name != "nil context" {
				ctx = context.Background()
			}
			if err := b.Run(ctx); err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("Run() = %v, want an error mentioning %q", err, tt.wantInErr)
			}
			if f.meCalls() != 0 || len(f.getCalls()) != 0 {
				t.Error("Run() called the Bot API despite the misuse")
			}
		})
	}
}

func TestBotRun(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		f := newFakeTelegram()
		f.me = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
		b, _ := newTestBot(t, f)
		if err := b.Run(context.Background()); !errors.Is(err, teleiq.ErrUnauthorized) || len(f.getCalls()) != 0 {
			t.Fatalf("Run() = %v after %d getUpdates, want Unauthorized before polling", err, len(f.getCalls()))
		}
	})

	t.Run("one run at a time, then again", func(t *testing.T) {
		f := newFakeTelegram()
		f.add(1, 1, 1)
		b, _ := newTestBot(t, f)
		handled := make(chan int64, 2)
		b.OnMessage(func(_ context.Context, c *Context) error { handled <- c.Update().UpdateID; return nil })

		stop := start(t, b)
		<-handled
		if err := b.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
			t.Errorf("second Run() = %v, want already running", err)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}

		f.add(2, 2, 1)
		stop = start(t, b)
		if id := <-handled; id != 2 {
			t.Errorf("after a restart handled update %d, want 2", id)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("polling options", func(t *testing.T) {
		tests := []struct {
			name        string
			opts        []Option
			wantTimeout int64
			wantAllowed string // as JSON: an empty list, unlike a missing one, resets the list of the token
		}{
			{"defaults of newTestBot", nil, 2, "[]"},
			{"no allowed updates", []Option{WithAllowedUpdates()}, 2, "[]"},
			{
				"timeout and allowed updates",
				[]Option{WithPollTimeout(5500 * time.Millisecond), WithAllowedUpdates("message", "callback_query")},
				5, `["message","callback_query"]`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				f := newFakeTelegram()
				b, _ := newTestBot(t, f, tt.opts...)
				stop := start(t, b)
				eventually(t, "a getUpdates", func() bool { return len(f.getCalls()) > 0 })
				if err := stop(); err != nil {
					t.Fatal(err)
				}
				for i, c := range f.getCalls() {
					allowed, _ := json.Marshal(c.allowed)
					if c.timeout != tt.wantTimeout || string(allowed) != tt.wantAllowed {
						t.Errorf("getUpdates %d: timeout %d, allowed_updates %s; want %d, %s", i, c.timeout, allowed, tt.wantTimeout, tt.wantAllowed)
					}
				}
			})
		}
	})
}
