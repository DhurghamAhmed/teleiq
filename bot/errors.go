package bot

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

// The errors of the webhook requests that WebhookHandler rejects.
var (
	// ErrBadSecret is a request answered 401 for a missing or wrong secret token.
	ErrBadSecret = errors.New("bot: webhook request without the right secret token")

	// ErrBadUpdate is a request answered 400 because its body is not one JSON update.
	ErrBadUpdate = errors.New("bot: webhook request body is not an update")

	// ErrBodyTooLarge is a request answered 413 because its body is too large.
	ErrBodyTooLarge = errors.New("bot: webhook request body is too large")

	// ErrQueueFull is an update answered 503 because the queue is full.
	ErrQueueFull = errors.New("bot: the queue of the chat or of the bot is full")

	// ErrNotRunning is an update answered 503 because RunWebhook is not running.
	ErrNotRunning = errors.New("bot: RunWebhook is not running")
)

// badSecretInterval is how often the default ErrorHandler logs ErrBadSecret at Warn.
const badSecretInterval = time.Minute

// newDefaultErrorHandler returns the ErrorHandler that logs to slog.Default.
func newDefaultErrorHandler(now func() time.Time) ErrorHandler {
	badSecret := &rateLimit{interval: badSecretInterval}
	return func(ctx context.Context, c *Context, err error) {
		msg := slog.String("error", sanitize.String(err.Error()))
		switch {
		case c != nil:
			slog.Default().LogAttrs(ctx, slog.LevelError, "bot: handler failed",
				slog.Int64("update_id", c.update.UpdateID), msg)
		case rejected(err):
			level := slog.LevelWarn
			if errors.Is(err, ErrBadSecret) && !badSecret.allow(now()) {
				level = slog.LevelDebug
			}
			slog.Default().LogAttrs(ctx, level, "bot: webhook request rejected", msg)
		default:
			slog.Default().LogAttrs(ctx, slog.LevelWarn, "bot: receiving updates failed", msg)
		}
	}
}

// rejected reports whether err is the rejection of a webhook request.
func rejected(err error) bool {
	for _, target := range []error{ErrBadSecret, ErrBadUpdate, ErrBodyTooLarge, ErrQueueFull, ErrNotRunning} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// rateLimit lets one event through per interval.
type rateLimit struct {
	interval time.Duration
	mu       sync.Mutex
	next     time.Time // when the next event may go through
}

func (l *rateLimit) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Before(l.next) {
		return false
	}
	l.next = now.Add(l.interval)
	return true
}
