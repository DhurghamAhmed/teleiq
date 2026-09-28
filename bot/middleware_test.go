package bot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// newDispatchBot returns a bot whose handler errors are appended to errs.
func newDispatchBot(t *testing.T) (*Bot, *[]error) {
	t.Helper()
	client, err := teleiq.NewClient(testToken, teleiq.WithTransport(teleiq.TransportFunc(
		func(context.Context, *teleiq.Request) (*teleiq.Response, error) {
			return nil, errors.New("no requests")
		})))
	if err != nil {
		t.Fatal(err)
	}
	var errs []error
	return New(client, WithErrorHandler(func(_ context.Context, _ *Context, err error) { errs = append(errs, err) })), &errs
}

func textUpdate(id int64, text string) *models.Update {
	return &models.Update{UpdateID: id, Message: &models.Message{MessageID: id, Date: 1, Chat: models.Chat{ID: 918273645, Type: "private"}, Text: &text}}
}

func trace(log *[]string, name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, c *Context) error {
			*log = append(*log, name+">")
			err := next(ctx, c)
			*log = append(*log, "<"+name)
			return err
		}
	}
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		update    *models.Update
		stop      bool // the second middleware returns an error without calling the rest
		wantLog   string
		wantError string
	}{
		{name: "the first added is the outermost", update: textUpdate(1, "hi"), wantLog: "A> B> handler <B <A"},
		{name: "updates without a handler go through it", update: &models.Update{UpdateID: 2, Poll: &models.Poll{ID: "p"}}, wantLog: "A> B> <B <A"},
		{name: "it can stop the handling", update: textUpdate(3, "hi"), stop: true, wantLog: "A> <A", wantError: "blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, errs := newDispatchBot(t)
			var log []string
			b.Use(trace(&log, "A"))
			if tt.stop {
				b.Use(func(Handler) Handler {
					return func(context.Context, *Context) error { return errors.New("blocked") }
				})
			} else {
				b.Use(trace(&log, "B"))
			}
			b.OnMessage(func(context.Context, *Context) error { log = append(log, "handler"); return nil })

			b.dispatch(context.Background(), tt.update)

			if got := strings.Join(log, " "); got != tt.wantLog {
				t.Errorf("ran %q, want %q", got, tt.wantLog)
			}
			if got := errorsText(*errs); got != tt.wantError {
				t.Errorf("errors %q, want %q", got, tt.wantError)
			}
		})
	}
}

func errorsText(errs []error) string {
	var s []string
	for _, err := range errs {
		s = append(s, err.Error())
	}
	return strings.Join(s, "; ")
}

func TestMiddlewareMisuse(t *testing.T) {
	b, errs := newDispatchBot(t)
	b.Use(func(Handler) Handler { return nil })
	b.OnMessage(func(context.Context, *Context) error { t.Error("the handler ran"); return nil })
	b.dispatch(context.Background(), textUpdate(1, "hi"))
	if got := errorsText(*errs); !strings.Contains(got, "nil handler") {
		t.Errorf("errors %q, want one about a nil handler", got)
	}

	b, _ = newDispatchBot(t)
	b.Use(Logging(), nil)
	if err := b.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "nil middleware") {
		t.Errorf("Run() = %v, want the nil middleware", err)
	}
}

// captureDefaultLog sends slog.Default to a buffer for the rest of the test.
func captureDefaultLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

var errCause = errors.New("cause")

func TestRecovery(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(b *Bot)
		wantError string
		wantIs    error
	}{
		{
			name:      "handler",
			setup:     func(b *Bot) { b.OnMessage(func(context.Context, *Context) error { panic("token " + testToken) }) },
			wantError: "bot: handler panicked: token <redacted>",
		},
		{
			name:      "error value",
			setup:     func(b *Bot) { b.OnMessage(func(context.Context, *Context) error { panic(errCause) }) },
			wantError: "bot: handler panicked: cause", wantIs: errCause,
		},
		{
			name: "filter",
			setup: func(b *Bot) {
				b.Handle(FilterFunc(func(*Context) bool { panic("bad filter") }), func(context.Context, *Context) error { return nil })
			},
			wantError: "bot: handler panicked: bad filter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureDefaultLog(t)
			b, errs := newDispatchBot(t)
			b.Use(Recovery())
			tt.setup(b)

			b.dispatch(context.Background(), textUpdate(1, "hi"))
			b.dispatch(context.Background(), textUpdate(2, "hi"))

			if len(*errs) != 2 {
				t.Fatalf("got %d errors, want one for each update", len(*errs))
			}
			var pe *PanicError
			if err := (*errs)[0]; !errors.As(err, &pe) || err.Error() != tt.wantError || len(pe.Stack) == 0 {
				t.Errorf("error = %v, want a *PanicError %q with a stack", err, tt.wantError)
			}
			if tt.wantIs != nil && !errors.Is((*errs)[0], tt.wantIs) {
				t.Errorf("errors.Is(%v, %v) = false, want true", (*errs)[0], tt.wantIs)
			}
			text := logs.String()
			if !strings.Contains(text, `msg="bot: handler panicked" update_id=1`) || !strings.Contains(text, "goroutine") {
				t.Errorf("logs lack the panic and its stack:\n%s", text)
			}
			if strings.Contains(text, testToken) {
				t.Errorf("logs contain the token:\n%s", text)
			}
		})
	}
}

func TestLoggingMiddleware(t *testing.T) {
	blocked := textUpdate(4, "blocked")
	tests := []struct {
		name   string
		update *models.Update
		want   []string // in the line logged
	}{
		{"handled", textUpdate(1, "secret text"), []string{
			`level=INFO msg="bot: update handled" update_id=1 kind=message duration=`, "handled=true\n",
		}},
		{"handler error", textUpdate(2, "fail"), []string{
			`update_id=2 kind=message duration=`, `handled=true error="failed with <redacted>"`,
		}},
		{"no handler", &models.Update{UpdateID: 3, CallbackQuery: &models.CallbackQuery{ID: "q", From: models.User{ID: 555666777}}}, []string{
			`level=INFO msg="bot: update handled" update_id=3 kind=callback_query duration=`, "handled=false\n",
		}},
		{"stopped by a later middleware", blocked, []string{
			`update_id=4 kind=message duration=`, `handled=false error=blocked`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureDefaultLog(t)
			b, _ := newDispatchBot(t)
			b.Use(Logging(), func(next Handler) Handler {
				return func(ctx context.Context, c *Context) error {
					if c.Update() == blocked {
						return errors.New("blocked")
					}
					return next(ctx, c)
				}
			})
			b.OnMessage(func(_ context.Context, c *Context) error {
				if *c.Message().Text == "fail" {
					return errors.New("failed with " + testToken)
				}
				return nil
			})

			b.dispatch(context.Background(), tt.update)

			text := logs.String()
			if strings.Count(text, "\n") != 1 {
				t.Errorf("logged %d lines, want 1:\n%s", strings.Count(text, "\n"), text)
			}
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("the log lacks %q:\n%s", want, text)
				}
			}
			for _, private := range []string{"secret text", "918273645", "555666777", testToken} {
				if strings.Contains(text, private) {
					t.Errorf("the log contains %q:\n%s", private, text)
				}
			}
		})
	}
}
