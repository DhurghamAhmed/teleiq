package teleiq

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

// RetryPolicy decides whether and when a failed request is sent again.
type RetryPolicy interface {
	// Retry returns the delay before the next attempt, or false to stop.
	Retry(attempt int, err error) (time.Duration, bool)
}

// Backoff is a RetryPolicy with exponential backoff and jitter.
type Backoff struct {
	MaxAttempts int           // attempts in total, the first one included; default 3
	BaseDelay   time.Duration // the delay before the second attempt; default 500ms
	MaxDelay    time.Duration // the longest delay; default 10s
}

// Retry returns a doubling, jittered delay until MaxAttempts is reached.
func (b Backoff) Retry(attempt int, _ error) (time.Duration, bool) {
	maxAttempts, base, maxDelay := b.MaxAttempts, b.BaseDelay, b.MaxDelay
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 10 * time.Second
	}
	if attempt >= maxAttempts {
		return 0, false
	}
	d := min(base, maxDelay)
	for range attempt - 1 {
		if d >= maxDelay/2 {
			d = maxDelay
			break
		}
		d *= 2
	}
	return d/2 + rand.N(d/2+1), true
}

// retryDelay returns the delay before retrying a failed attempt, or false to stop.
func (c *Client) retryDelay(ctx context.Context, attempt int, err error, network bool) (time.Duration, bool) {
	if ctx.Err() != nil {
		return 0, false
	}
	var floor time.Duration
	var apiErr *Error
	switch {
	case network:
	case errors.As(err, &apiErr) && apiErr.ErrorCode == http.StatusTooManyRequests:
		if p := apiErr.Parameters; p != nil && p.RetryAfter != nil {
			floor = seconds(int64(*p.RetryAfter))
		}
	case errors.As(err, &apiErr) && apiErr.ErrorCode >= 500:
	default:
		return 0, false
	}
	delay, ok := c.retry.Retry(attempt, err)
	if !ok {
		return 0, false
	}
	delay = max(delay, floor)
	// Waiting past the deadline would only return the same error later.
	if deadline, has := ctx.Deadline(); has && time.Until(deadline) < delay {
		return 0, false
	}
	return delay, true
}

// sleep waits for d, returning false if ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
