package bot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/DhurghamAhmed/teleiq/models"
)

const secretHeader = "X-Telegram-Bot-Api-Secret-Token"

// webhook is the state of a running RunWebhook, which the handler feeds.
type webhook struct {
	sched *scheduler

	// mu orders the end of each handler against gaveUp.
	mu     sync.Mutex
	gaveUp chan struct{} // closed when RunWebhook gives up at the shutdown timeout
}

// WithSecretToken makes the webhook accept only requests with the secret token.
func WithSecretToken(token string) Option {
	return func(c *config) error {
		if !validSecret(token) {
			return errors.New("bot: WithSecretToken: the token has 1 to 256 letters, digits, _ or -")
		}
		sum := sha256.Sum256([]byte(token))
		c.secret = sum[:]
		return nil
	}
}

func validSecret(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// WithMaxBodySize limits the size of a webhook request; larger ones get 413.
func WithMaxBodySize(n int64) Option {
	return func(c *config) error {
		if n < 1 {
			return errors.New("bot: WithMaxBodySize: the size must be positive")
		}
		c.maxBody = n
		return nil
	}
}

// WithAsyncWebhook makes the webhook answer once an update is queued, not handled.
func WithAsyncWebhook() Option {
	return func(c *config) error {
		c.async = true
		return nil
	}
}

// WebhookHandler returns the http.Handler that receives updates from Telegram.
func (b *Bot) WebhookHandler() http.Handler {
	return http.HandlerFunc(b.serveWebhook)
}

func (b *Bot) serveWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.cfg.secret != nil {
		// Comparing digests keeps the length of the secret from showing in the timing.
		sum := sha256.Sum256([]byte(r.Header.Get(secretHeader)))
		if subtle.ConstantTimeCompare(sum[:], b.cfg.secret) != 1 {
			b.reject(w, r, http.StatusUnauthorized, "unauthorized", ErrBadSecret)
			return
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, b.cfg.maxBody))
	if tooLarge, ok := errors.AsType[*http.MaxBytesError](err); ok {
		b.reject(w, r, http.StatusRequestEntityTooLarge, "request too large",
			fmt.Errorf("%w: the limit is %d bytes", ErrBodyTooLarge, tooLarge.Limit))
		return
	}
	var u models.Update
	// The error of json.Unmarshal can quote the body, so only the sentinel is reported.
	if err != nil || !shallowObject(body) || json.Unmarshal(body, &u) != nil {
		b.reject(w, r, http.StatusBadRequest, "not an update", ErrBadUpdate)
		return
	}
	hook := b.hook.Load()
	if hook == nil {
		b.reject(w, r, http.StatusServiceUnavailable, "not running", ErrNotRunning)
		return
	}
	j, done := hook.job(&u, b.cfg.async)
	if err := hook.sched.trySubmit(j); err != nil {
		// A full queue or a shutdown: Telegram sends the update again later.
		if !errors.Is(err, ErrQueueFull) {
			err = ErrNotRunning
		}
		b.reject(w, r, http.StatusServiceUnavailable, "busy", err)
		return
	}
	if b.cfg.async {
		return
	}
	if err := hook.wait(r.Context(), done); err != nil {
		b.reject(w, r, http.StatusServiceUnavailable, "stopped", err)
	}
}

// job returns the job of u and, unless async, a channel closed when it is handled.
func (h *webhook) job(u *models.Update, async bool) (job, <-chan struct{}) {
	if async {
		return job{update: u}, nil
	}
	done := make(chan struct{})
	return job{update: u, done: func(*models.Update) {
		h.mu.Lock()
		defer h.mu.Unlock()
		select {
		case <-h.gaveUp:
		default:
			close(done)
		}
	}}, done
}

// wait blocks until the update is handled, ctx ends, or RunWebhook gives up on it.
func (h *webhook) wait(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
	case <-h.gaveUp:
		// Both can be ready, and select picks either; done is final once gaveUp is closed.
		select {
		case <-done:
		default:
			return ErrNotRunning
		}
	case <-ctx.Done():
	}
	return nil
}

// giveUp marks the updates whose handler has not returned as given up.
func (h *webhook) giveUp() {
	h.mu.Lock()
	defer h.mu.Unlock()
	close(h.gaveUp)
}

// reject answers a request with code and reports err to the ErrorHandler.
func (b *Bot) reject(w http.ResponseWriter, r *http.Request, code int, text string, err error) {
	http.Error(w, text, code)
	b.cfg.errorHandler(r.Context(), nil, err)
}

// maxDepth limits how deeply a webhook request nests objects and arrays.
const maxDepth = 128

// shallowObject reports whether data is a JSON object no deeper than maxDepth.
func shallowObject(data []byte) bool {
	data = bytes.TrimLeft(data, " \t\r\n")
	if len(data) == 0 || data[0] != '{' {
		return false
	}
	depth, inString, escaped := 0, false, false
	for _, c := range data {
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{' || c == '[':
			if depth++; depth > maxDepth {
				return false
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return true
}

// RunWebhook handles the updates that WebhookHandler receives until ctx ends.
func (b *Bot) RunWebhook(ctx context.Context) error {
	if ok, err := b.begin(ctx, "RunWebhook"); !ok {
		return err
	}
	defer b.running.Store(false)

	handlerCtx, cancelHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHandlers()
	hook := &webhook{sched: newScheduler(b.cfg.workers, b.cfg.queueSize), gaveUp: make(chan struct{})}
	hook.sched.start(handlerCtx, b.dispatch)
	b.hook.Store(hook)
	defer b.hook.CompareAndSwap(hook, nil)

	<-ctx.Done()
	hook.sched.close()
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), b.cfg.poll.shutdownTimeout)
	defer cancel()
	if err := hook.sched.wait(drain); err != nil {
		// Give up before cancelHandlers so canceled handlers are not taken as handled.
		hook.giveUp()
		return ErrShutdownTimeout
	}
	return nil
}
