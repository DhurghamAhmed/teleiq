package teleiq

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

// RateLimiter paces requests before they are sent.
type RateLimiter interface {
	// Wait blocks until a request to method for chat may be sent, or returns an error.
	Wait(ctx context.Context, method string, chat models.ChatID) error
}

// targeter is implemented by the generated parameters that have a chat_id.
type targeter interface {
	targetChat() models.ChatID
}

func targetOf(params any) models.ChatID {
	if t, ok := params.(targeter); ok {
		return t.targetChat()
	}
	return models.ChatID{}
}

// cooldowns remembers the chats under flood control and until when.
type cooldowns struct {
	mu    sync.Mutex
	until map[models.ChatID]time.Time
}

// wait blocks until chat is out of flood control and returns how long it waited.
func (c *cooldowns) wait(ctx context.Context, method string, chat models.ChatID) (time.Duration, error) {
	c.mu.Lock()
	until, ok := c.until[chat]
	c.mu.Unlock()
	left := time.Until(until)
	if !ok || left <= 0 {
		return 0, nil
	}
	if deadline, has := ctx.Deadline(); has && deadline.Before(until) {
		secs := int(math.Ceil(left.Seconds()))
		return 0, &Error{
			ErrorCode:   http.StatusTooManyRequests,
			Description: "Too Many Requests: flood control ends after the deadline; retry after " + strconv.Itoa(secs),
			Parameters:  &models.ResponseParameters{RetryAfter: &secs},
			Method:      method,
		}
	}
	start := time.Now()
	if !sleep(ctx, left) {
		return time.Since(start), ctx.Err()
	}
	return time.Since(start), nil
}

// block keeps requests away from chat for d and forgets expired chats.
func (c *cooldowns) block(chat models.ChatID, d time.Duration) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, until := range c.until {
		if !until.After(now) {
			delete(c.until, k)
		}
	}
	if until := now.Add(d); until.After(c.until[chat]) {
		if c.until == nil {
			c.until = map[models.ChatID]time.Time{}
		}
		c.until[chat] = until
	}
}

// seconds converts a number of seconds to a Duration without overflowing.
func seconds(n int64) time.Duration {
	switch {
	case n <= 0:
		return 0
	case n > int64(math.MaxInt64/time.Second):
		return math.MaxInt64
	}
	return time.Duration(n) * time.Second
}

// retryAfterHeader reads a Retry-After header, in seconds or as an HTTP date.
func retryAfterHeader(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		return seconds(secs)
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0)
	}
	return 0
}

// withRetryAfter returns a copy of p with RetryAfter set to d, rounded up to seconds.
func withRetryAfter(p *models.ResponseParameters, d time.Duration) *models.ResponseParameters {
	out := models.ResponseParameters{}
	if p != nil {
		out = *p
	}
	secs := int(min(math.Ceil(d.Seconds()), math.MaxInt32))
	out.RetryAfter = &secs
	return &out
}

// NewRateLimiter returns a RateLimiter that paces requests overall and per chat.
func NewRateLimiter(perSecond int, chatEvery, groupEvery time.Duration) RateLimiter {
	l := &paceLimiter{chatEvery: max(chatEvery, 0), groupEvery: max(groupEvery, 0), next: map[models.ChatID]time.Time{}}
	if perSecond > 0 {
		l.interval = time.Second / time.Duration(perSecond)
		// A second's worth of requests may go at once if the rate holds over time.
		l.burst = time.Duration(perSecond-1) * l.interval
	}
	return l
}

type paceLimiter struct {
	interval   time.Duration // between two requests in all; 0 for no limit
	burst      time.Duration // how far ahead of the rate a burst may go
	chatEvery  time.Duration
	groupEvery time.Duration

	mu    sync.Mutex
	due   time.Time                   // when the rate allows the next request, bursts aside
	next  map[models.ChatID]time.Time // when each chat may get its next request
	swept int                         // the size of next after it was last swept
}

func (l *paceLimiter) Wait(ctx context.Context, method string, chat models.ChatID) error {
	if chat == (models.ChatID{}) {
		return nil
	}
	every := l.chatEvery
	if chat.ID() < 0 || chat.Username() != "" {
		every = l.groupEvery
	}
	if every > 0 {
		l.mu.Lock()
		now := time.Now()
		at := later(now, l.next[chat])
		if afterDeadline(ctx, at) {
			l.mu.Unlock()
			return lateError(method)
		}
		l.next[chat] = at.Add(every)
		l.sweep(now)
		l.mu.Unlock()
		if err := waitUntil(ctx, at); err != nil {
			return err
		}
	}
	if l.interval > 0 {
		l.mu.Lock()
		at := later(time.Now(), l.due.Add(-l.burst))
		if afterDeadline(ctx, at) {
			l.mu.Unlock()
			return lateError(method)
		}
		l.due = later(l.due, at).Add(l.interval)
		l.mu.Unlock()
		return waitUntil(ctx, at)
	}
	return nil
}

// sweep forgets the chats whose turn has come once the map has grown large.
func (l *paceLimiter) sweep(now time.Time) {
	if len(l.next) <= 2*l.swept+1024 {
		return
	}
	for chat, t := range l.next {
		if !t.After(now) {
			delete(l.next, chat)
		}
	}
	l.swept = len(l.next)
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func afterDeadline(ctx context.Context, at time.Time) bool {
	deadline, ok := ctx.Deadline()
	return ok && deadline.Before(at)
}

func lateError(method string) error {
	return fmt.Errorf("the turn of %s comes after the deadline: %w", method, context.DeadlineExceeded)
}

// waitUntil waits until at, or returns the error of ctx if it ends first.
func waitUntil(ctx context.Context, at time.Time) error {
	if d := time.Until(at); d > 0 && !sleep(ctx, d) {
		return ctx.Err()
	}
	return nil
}
