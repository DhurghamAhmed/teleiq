package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

type report struct {
	c   *Context
	err error
}

func (r report) String() string { return fmt.Sprintf("{Context: %v, err: %v}", r.c != nil, r.err) }

// reports records what reaches an ErrorHandler.
type reports struct {
	mu   sync.Mutex
	list []report
}

func (r *reports) handle(_ context.Context, c *Context, err error) {
	r.mu.Lock()
	r.list = append(r.list, report{c, err})
	r.mu.Unlock()
}

func (r *reports) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.list)
}

// since returns the reports after the first n.
func (r *reports) since(n int) []report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]report(nil), r.list[n:]...)
}

func TestWebhookUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		reject  func(t *testing.T, b *Bot, post func(body, secret string) int, started chan struct{}) int
		wantErr error
	}{
		{
			name: "before RunWebhook",
			reject: func(_ *testing.T, _ *Bot, post func(string, string) int, _ chan struct{}) int {
				return post(update(1, 1), "")
			},
			wantErr: ErrNotRunning,
		},
		{
			name: "after RunWebhook",
			reject: func(t *testing.T, b *Bot, post func(string, string) int, _ chan struct{}) int {
				if err := startWebhook(t, b)(); err != nil {
					t.Fatal(err)
				}
				return post(update(1, 1), "")
			},
			wantErr: ErrNotRunning,
		},
		{
			name: "while stopping",
			reject: func(t *testing.T, b *Bot, post func(string, string) int, started chan struct{}) int {
				stop := startWebhook(t, b)
				hook := b.hook.Load()
				postAsync(post, update(1, 1))
				<-started // the update in progress keeps RunWebhook from returning
				stopped := make(chan error, 1)
				go func() { stopped <- stop() }()
				t.Cleanup(func() { <-stopped }) // after the update in progress is released
				eventually(t, "the queues to close", func() bool {
					hook.sched.mu.Lock()
					defer hook.sched.mu.Unlock()
					return hook.sched.closed
				})
				return post(update(2, 1), "")
			},
			wantErr: ErrNotRunning,
		},
		{
			name: "gave up at the shutdown timeout",
			opts: []Option{WithShutdownTimeout(20 * time.Millisecond)},
			reject: func(t *testing.T, b *Bot, post func(string, string) int, started chan struct{}) int {
				stop := startWebhook(t, b)
				code := postAsync(post, update(1, 1))
				<-started
				if err := stop(); !errors.Is(err, ErrShutdownTimeout) {
					t.Errorf("RunWebhook() = %v, want ErrShutdownTimeout", err)
				}
				return <-code
			},
			wantErr: ErrNotRunning,
		},
		{
			name: "full queue",
			opts: []Option{WithAsyncWebhook(), WithWorkers(1), WithQueueSize(1)},
			reject: func(t *testing.T, b *Bot, post func(string, string) int, started chan struct{}) int {
				stop := startWebhook(t, b)
				t.Cleanup(func() { _ = stop() }) // after the update in progress is released
				post(update(1, 1), "")
				<-started // the worker holds update 1 and the queue has room for update 2
				if c := post(update(2, 1), ""); c != http.StatusOK {
					t.Errorf("queued update: %d, want 200", c)
				}
				return post(update(3, 1), "")
			},
			wantErr: ErrQueueFull,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec reports
			b, post, started, release := blockingBot(t, append(tt.opts, WithErrorHandler(rec.handle))...)
			defer close(release)
			if code := tt.reject(t, b, post, started); code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", code)
			}
			got := rec.since(0)
			if len(got) != 1 || got[0].c != nil || !errors.Is(got[0].err, tt.wantErr) {
				t.Errorf("reported %v, want one %v with a nil Context", got, tt.wantErr)
			}
		})
	}
}

// captureLogAt sends slog.Default, from level up, to a buffer for the rest of the test.
func captureLogAt(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestDefaultErrorHandler(t *testing.T) {
	b, _ := newDispatchBot(t)
	c := b.NewContext(&models.Update{UpdateID: 7})
	tests := []struct {
		name string
		c    *Context
		err  error
		want string
	}{
		{"handler", c, errors.New("failed with " + testToken), `level=ERROR msg="bot: handler failed" update_id=7 error="failed with <redacted>"`},
		{"receiving", nil, errors.New("teleiq: getUpdates: timeout"), `level=WARN msg="bot: receiving updates failed" error="teleiq: getUpdates: timeout"`},
		{"bad update", nil, ErrBadUpdate, `level=WARN msg="bot: webhook request rejected" error="bot: webhook request body is not an update"`},
		{"too large", nil, fmt.Errorf("%w: the limit is 10 bytes", ErrBodyTooLarge), `level=WARN msg="bot: webhook request rejected" error="bot: webhook request body is too large: the limit is 10 bytes"`},
		{"queue full", nil, ErrQueueFull, `level=WARN msg="bot: webhook request rejected" error="bot: the queue of the chat or of the bot is full"`},
		{"not running", nil, ErrNotRunning, `level=WARN msg="bot: webhook request rejected" error="bot: RunWebhook is not running"`},
		{"bad secret", nil, ErrBadSecret, `level=WARN msg="bot: webhook request rejected" error="bot: webhook request without the right secret token"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogAt(t, slog.LevelDebug)
			newDefaultErrorHandler(time.Now)(context.Background(), tt.c, tt.err)
			if got := strings.TrimSpace(logs.String()); !strings.HasSuffix(got, tt.want) || strings.Count(got, "\n") != 0 {
				t.Errorf("logged %q, want one line ending with %q", got, tt.want)
			}
		})
	}
}

func TestDefaultErrorHandlerLimitsBadSecrets(t *testing.T) {
	logs := captureLogAt(t, slog.LevelDebug)
	clock := &fakeClock{t: time.Unix(1e9, 0)}
	handle := newDefaultErrorHandler(clock.now)
	steps := []struct {
		after time.Duration
		err   error
		want  string // the level logged
	}{
		{0, ErrBadSecret, "WARN"},
		{0, ErrBadSecret, "DEBUG"},
		{0, ErrBadUpdate, "WARN"}, // other rejections are not limited
		{time.Minute - time.Nanosecond, ErrBadSecret, "DEBUG"},
		{time.Nanosecond, fmt.Errorf("wrapped: %w", ErrBadSecret), "WARN"},
		{time.Second, ErrBadSecret, "DEBUG"},
	}
	for i, s := range steps {
		clock.advance(s.after)
		logs.Reset()
		handle(context.Background(), nil, s.err)
		if want := "level=" + s.want + ` msg="bot: webhook request rejected"`; !strings.Contains(logs.String(), want) {
			t.Errorf("step %d: logged %q, want %s", i, logs.String(), want)
		}
	}

	// Each bot has its own limit.
	logs.Reset()
	newDefaultErrorHandler(clock.now)(context.Background(), nil, ErrBadSecret)
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("a new bot logged %q, want WARN", logs.String())
	}
}
