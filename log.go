package teleiq

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

// Logs hold methods, attempts, durations and errors, never tokens or user data.

func (c *Client) logAttempt(ctx context.Context, method string, attempt int, took time.Duration, err error) {
	if !c.log.Enabled(ctx, slog.LevelDebug) {
		return
	}
	attrs := []slog.Attr{slog.String("method", method), slog.Int("attempt", attempt), slog.Duration("duration", took)}
	c.log.LogAttrs(ctx, slog.LevelDebug, "teleiq: request", append(attrs, errorAttrs(err)...)...)
}

func (c *Client) logRetry(ctx context.Context, method string, attempt int, delay time.Duration, err error) {
	if !c.log.Enabled(ctx, slog.LevelWarn) {
		return
	}
	attrs := []slog.Attr{slog.String("method", method), slog.Int("attempt", attempt), slog.Duration("delay", delay)}
	c.log.LogAttrs(ctx, slog.LevelWarn, "teleiq: retrying request", append(attrs, errorAttrs(err)...)...)
}

func (c *Client) logFloodWait(ctx context.Context, method string, waited time.Duration) {
	c.log.LogAttrs(ctx, slog.LevelWarn, "teleiq: waited for flood control",
		slog.String("method", method), slog.Duration("duration", waited))
}

func (c *Client) logDownload(ctx context.Context, filePath string, took time.Duration, err error) {
	if !c.log.Enabled(ctx, slog.LevelDebug) {
		return
	}
	attrs := []slog.Attr{slog.String("file_path", sanitize.String(filePath)), slog.Duration("duration", took)}
	c.log.LogAttrs(ctx, slog.LevelDebug, "teleiq: download", append(attrs, errorAttrs(err)...)...)
}

func errorAttrs(err error) []slog.Attr {
	if err == nil {
		return nil
	}
	var attrs []slog.Attr
	var apiErr *Error
	if errors.As(err, &apiErr) {
		attrs = append(attrs, slog.Int("error_code", apiErr.ErrorCode))
	}
	return append(attrs, slog.String("error", sanitize.String(err.Error())))
}
