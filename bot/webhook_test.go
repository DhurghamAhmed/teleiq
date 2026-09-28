package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newWebhookBot returns a bot served by a test server, and a function that posts a body to it.
func newWebhookBot(t *testing.T, opts ...Option) (*Bot, func(body, secret string) int) {
	t.Helper()
	b, _ := newTestBot(t, newFakeTelegram(), opts...)
	srv := httptest.NewServer(b.WebhookHandler())
	t.Cleanup(srv.Close)
	return b, func(body, secret string) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if secret != "" {
			req.Header.Set(secretHeader, secret)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Errorf("POST: %v", err)
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
}

// startWebhook runs RunWebhook and returns a function that stops it and returns its result.
func startWebhook(t testing.TB, b *Bot) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- b.RunWebhook(ctx) }()
	eventually(t, "RunWebhook to start", func() bool { return b.hook.Load() != nil })
	return func() error {
		cancel()
		return <-result
	}
}

func update(id, chat int64) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1,"chat":{"id":%d,"type":"private"},"text":"hi"}}`, id, id, chat)
}

func TestWebhookLifecycle(t *testing.T) {
	b, post := newWebhookBot(t)
	b.OnMessage(func(context.Context, *Context) error { return nil })
	if code := post(update(1, 1), ""); code != http.StatusServiceUnavailable {
		t.Errorf("before RunWebhook: %d, want 503", code)
	}
	stop := startWebhook(t, b)
	if code := post(update(2, 1), ""); code != http.StatusOK {
		t.Errorf("while running: %d, want 200", code)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if code := post(update(3, 1), ""); code != http.StatusServiceUnavailable {
		t.Errorf("after RunWebhook: %d, want 503", code)
	}
}

func TestWebhookResponses(t *testing.T) {
	const secret, wrongSecret = "s3cret_token-1", "s3cret_token-2"
	tests := []struct {
		name, body, secret string
		want               int
		wantErr            error // reported to the ErrorHandler with a nil Context
		wantHandlerErr     bool  // reported with the Context of the update
	}{
		{name: "update", body: update(1, 1), secret: secret, want: http.StatusOK},
		{name: "handler error", body: update(2, 1), secret: secret, want: http.StatusOK, wantHandlerErr: true},
		{name: "no secret", body: update(3, 1), want: http.StatusUnauthorized, wantErr: ErrBadSecret},
		{name: "wrong secret", body: update(4, 1), secret: wrongSecret, want: http.StatusUnauthorized, wantErr: ErrBadSecret},
		{name: "not an update", body: "not json", secret: secret, want: http.StatusBadRequest, wantErr: ErrBadUpdate},
		{name: "not an object", body: "null", secret: secret, want: http.StatusBadRequest, wantErr: ErrBadUpdate},
		{name: "wrong type", body: `{"update_id":"private text"}`, secret: secret, want: http.StatusBadRequest, wantErr: ErrBadUpdate},
		{name: "trailing data", body: update(6, 1) + " {}", secret: secret, want: http.StatusBadRequest, wantErr: ErrBadUpdate},
		{name: "nested too deeply", body: `{"update_id":7,"x":` + strings.Repeat("[", maxDepth) + strings.Repeat("]", maxDepth) + `}`, secret: secret, want: http.StatusBadRequest, wantErr: ErrBadUpdate},
		{name: "too large", body: `{"update_id":5,"padding":"` + strings.Repeat("x", 2000) + `"}`, secret: secret, want: http.StatusRequestEntityTooLarge, wantErr: ErrBodyTooLarge},
	}
	var rec reports
	b, post := newWebhookBot(t, WithSecretToken(secret), WithMaxBodySize(1000), WithErrorHandler(rec.handle))
	b.OnMessage(func(_ context.Context, c *Context) error {
		if c.Update().UpdateID == 2 {
			return errors.New("failed")
		}
		return nil
	})
	stop := startWebhook(t, b)
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := rec.len()
			if code := post(tt.body, tt.secret); code != tt.want {
				t.Errorf("status = %d, want %d", code, tt.want)
			}
			got := rec.since(before)
			switch {
			case tt.wantErr != nil:
				if len(got) != 1 || got[0].c != nil || !errors.Is(got[0].err, tt.wantErr) {
					t.Fatalf("reported %v, want one %v with a nil Context", got, tt.wantErr)
				}
				for _, private := range []string{secret, wrongSecret, testToken, tt.body, "private text"} {
					if strings.Contains(got[0].err.Error(), private) {
						t.Errorf("the error %q contains %q", got[0].err, private)
					}
				}
			case tt.wantHandlerErr:
				if len(got) != 1 || got[0].c == nil || got[0].c.Update().UpdateID != 2 {
					t.Errorf("reported %v, want the error of update 2 with its Context", got)
				}
			case len(got) != 0:
				t.Errorf("reported %v, want nothing", got)
			}
		})
	}
}

func TestWebhookMethod(t *testing.T) {
	var reported reports
	b, _ := newTestBot(t, newFakeTelegram(), WithErrorHandler(reported.handle))
	rec := httptest.NewRecorder()
	b.WebhookHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Errorf("GET = %d with Allow %q, want 405 with POST", rec.Code, rec.Header().Get("Allow"))
	}
	if n := reported.len(); n != 0 {
		t.Errorf("%d errors reported, want none: only POST requests can carry updates", n)
	}
}

// blockingBot returns a bot whose handler signals started and waits for release.
func blockingBot(t *testing.T, opts ...Option) (*Bot, func(body, secret string) int, chan struct{}, chan struct{}) {
	t.Helper()
	b, post := newWebhookBot(t, opts...)
	started, release := make(chan struct{}, 16), make(chan struct{})
	b.OnMessage(func(context.Context, *Context) error {
		started <- struct{}{}
		<-release
		return nil
	})
	return b, post, started, release
}

// postAsync posts in the background and returns where the status arrives.
func postAsync(post func(body, secret string) int, body string) <-chan int {
	code := make(chan int, 1)
	go func() { code <- post(body, "") }()
	return code
}

func TestWebhookModes(t *testing.T) {
	t.Run("sync answers once the handler returns", func(t *testing.T) {
		b, post, started, release := blockingBot(t)
		stop := startWebhook(t, b)
		code := postAsync(post, update(1, 1))
		<-started
		select {
		case c := <-code:
			t.Fatalf("answered %d while the handler ran", c)
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if c := <-code; c != http.StatusOK {
			t.Errorf("status = %d, want 200", c)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("async answers once the update is queued", func(t *testing.T) {
		b, post, started, release := blockingBot(t, WithAsyncWebhook())
		stop := startWebhook(t, b)
		if c := post(update(1, 1), ""); c != http.StatusOK {
			t.Errorf("status = %d, want 200", c)
		}
		<-started
		close(release)
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("full queue", func(t *testing.T) {
		b, post, started, release := blockingBot(t, WithAsyncWebhook(), WithWorkers(1), WithQueueSize(1))
		stop := startWebhook(t, b)
		post(update(1, 1), "")
		<-started // the worker holds update 1 and the queue has room for update 2
		if c := post(update(2, 1), ""); c != http.StatusOK {
			t.Errorf("queued update: %d, want 200", c)
		}
		if c := post(update(3, 1), ""); c != http.StatusServiceUnavailable {
			t.Errorf("update past a full queue: %d, want 503", c)
		}
		close(release)
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWebhookShutdown(t *testing.T) {
	t.Run("drains the updates in progress", func(t *testing.T) {
		b, post, started, release := blockingBot(t)
		stop := startWebhook(t, b)
		inflight := postAsync(post, update(1, 1))
		<-started
		stopped := make(chan error, 1)
		go func() { stopped <- stop() }()
		eventually(t, "new requests to be refused", func() bool { return post(update(2, 2), "") == http.StatusServiceUnavailable })
		close(release)
		if c := <-inflight; c != http.StatusOK {
			t.Errorf("request in progress: %d, want 200", c)
		}
		if err := <-stopped; err != nil {
			t.Errorf("RunWebhook() = %v, want nil", err)
		}
	})

	t.Run("gives up at the timeout", func(t *testing.T) {
		b, post, started, release := blockingBot(t, WithShutdownTimeout(50*time.Millisecond))
		defer close(release)
		stop := startWebhook(t, b)
		inflight := postAsync(post, update(1, 1))
		<-started
		if err := stop(); !errors.Is(err, ErrShutdownTimeout) {
			t.Errorf("RunWebhook() = %v, want ErrShutdownTimeout", err)
		}
		if c := <-inflight; c != http.StatusServiceUnavailable {
			t.Errorf("request in progress: %d, want 503 so that Telegram sends it again", c)
		}
	})
}

func TestWebhookKeepsTheOrderOfArrival(t *testing.T) {
	b, post := newWebhookBot(t, WithAsyncWebhook())
	var mu sync.Mutex
	var order []int64
	b.OnMessage(func(_ context.Context, c *Context) error {
		mu.Lock()
		order = append(order, c.Update().UpdateID)
		mu.Unlock()
		return nil
	})
	stop := startWebhook(t, b)
	for id := int64(20); id >= 1; id-- { // update_ids arrive in reverse
		if c := post(update(id, 1), ""); c != http.StatusOK {
			t.Fatalf("status = %d", c)
		}
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for i, id := range order {
		if id != int64(20-i) {
			t.Fatalf("handled %v, want the order of arrival", order)
		}
	}
}

func TestWebhookMisuse(t *testing.T) {
	tests := []struct {
		name      string
		opts      []Option
		wantInErr string
	}{
		{"empty secret", []Option{WithSecretToken("")}, "WithSecretToken"},
		{"secret with a space", []Option{WithSecretToken("bad token")}, "WithSecretToken"},
		{"long secret", []Option{WithSecretToken(strings.Repeat("a", 257))}, "WithSecretToken"},
		{"no body", []Option{WithMaxBodySize(0)}, "WithMaxBodySize"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBot(t, newFakeTelegram(), tt.opts...)
			if err := b.RunWebhook(context.Background()); err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("RunWebhook() = %v, want an error mentioning %q", err, tt.wantInErr)
			}
		})
	}

	t.Run("one run at a time", func(t *testing.T) {
		b, _ := newTestBot(t, newFakeTelegram())
		stop := startWebhook(t, b)
		if err := b.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
			t.Errorf("Run() during RunWebhook = %v, want already running", err)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	})
}
