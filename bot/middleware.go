package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

// Middleware wraps the handling of an update.
type Middleware func(next Handler) Handler

// Use adds middleware around the handling of every update.
func (b *Bot) Use(mw ...Middleware) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range mw {
		if m == nil {
			b.misuse(errors.New("bot: Use: nil middleware"))
			return
		}
	}
	b.middleware = append(b.middleware, mw...)
}

// PanicError is the error that Recovery reports for a panicking handler or filter.
type PanicError struct {
	Value any    // the value given to panic
	Stack []byte // the stack of the goroutine that panicked
}

// Error returns the panic value as text, without the bot token.
func (e *PanicError) Error() string {
	return sanitize.String(fmt.Sprintf("bot: handler panicked: %v", e.Value))
}

// Unwrap returns the panic value when it is an error.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// Recovery returns middleware that turns a panic in a handler into a *PanicError.
func Recovery() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, c *Context) (err error) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				pe := &PanicError{Value: v, Stack: debug.Stack()}
				slog.Default().LogAttrs(ctx, slog.LevelError, "bot: handler panicked",
					slog.Int64("update_id", c.update.UpdateID),
					slog.String("panic", sanitize.String(fmt.Sprint(v))),
					slog.String("stack", sanitize.String(string(pe.Stack))))
				err = pe
			}()
			return next(ctx, c)
		}
	}
}

// Logging returns middleware that logs every update through slog.Default.
func Logging() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, c *Context) error {
			start := time.Now()
			err := next(ctx, c)
			attrs := []slog.Attr{
				slog.Int64("update_id", c.update.UpdateID),
				slog.String("kind", updateKind(c.update)),
				slog.Duration("duration", time.Since(start)),
				slog.Bool("handled", c.handled),
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", sanitize.String(err.Error())))
			}
			slog.Default().LogAttrs(ctx, slog.LevelInfo, "bot: update handled", attrs...)
			return err
		}
	}
}
