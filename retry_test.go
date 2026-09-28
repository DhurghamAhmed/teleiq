package teleiq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/upload"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestBackoff(t *testing.T) {
	type window struct{ min, max time.Duration }
	tests := []struct {
		name  string
		b     Backoff
		tries map[int]window // attempt -> allowed delays; a missing attempt stops
		stop  int
	}{
		{
			name:  "defaults",
			tries: map[int]window{1: {250 * time.Millisecond, 500 * time.Millisecond}, 2: {500 * time.Millisecond, time.Second}},
			stop:  3,
		},
		{
			name:  "negative fields take the defaults",
			b:     Backoff{MaxAttempts: -1, BaseDelay: -1, MaxDelay: -1},
			tries: map[int]window{1: {250 * time.Millisecond, 500 * time.Millisecond}},
			stop:  3,
		},
		{name: "retries off", b: Backoff{MaxAttempts: 1}, stop: 1},
		{
			name: "capped",
			b:    Backoff{MaxAttempts: 10, BaseDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond},
			tries: map[int]window{
				1: {50 * time.Millisecond, 100 * time.Millisecond},
				2: {100 * time.Millisecond, 200 * time.Millisecond},
				3: {150 * time.Millisecond, 300 * time.Millisecond},
				9: {150 * time.Millisecond, 300 * time.Millisecond},
			},
			stop: 10,
		},
		{
			name:  "many attempts do not overflow",
			b:     Backoff{MaxAttempts: 1 << 30, BaseDelay: time.Hour, MaxDelay: 1 << 62},
			tries: map[int]window{1 << 29: {1 << 61, 1 << 62}},
			stop:  1 << 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for attempt, w := range tt.tries {
				for range 200 {
					d, ok := tt.b.Retry(attempt, nil)
					if !ok || d < w.min || d > w.max {
						t.Fatalf("Retry(%d) = %v, %v; want a delay in [%v, %v]", attempt, d, ok, w.min, w.max)
					}
				}
			}
			if d, ok := tt.b.Retry(tt.stop, nil); ok {
				t.Errorf("Retry(%d) = %v, true; want to stop", tt.stop, d)
			}
		})
	}
}

// step is one scripted answer of the transport.
type step struct {
	status   int
	reply    string
	err      error
	readBody bool // read the request body before answering
}

type scriptedTransport struct {
	mu     sync.Mutex
	steps  []step
	bodies []string // the uploaded document of each call, or the JSON body
}

func (s *scriptedTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	s.mu.Lock()
	st := s.steps[min(len(s.bodies), len(s.steps)-1)]
	s.bodies = append(s.bodies, "")
	n := len(s.bodies) - 1
	s.mu.Unlock()
	if st.readBody && req.Body != nil {
		s.bodies[n] = readDocument(req)
	}
	if st.err != nil {
		return nil, st.err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &Response{StatusCode: st.status, Body: []byte(st.reply)}, nil
}

func (s *scriptedTransport) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

// readDocument returns the uploaded document of a multipart request, or the whole body otherwise.
func readDocument(req *Request) string {
	mediaType, ps, _ := mime.ParseMediaType(req.ContentType)
	if mediaType != "multipart/form-data" {
		data, _ := io.ReadAll(req.Body)
		return string(data)
	}
	doc := ""
	mr := multipart.NewReader(req.Body, ps["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			return doc
		}
		if data, _ := io.ReadAll(part); part.FormName() == "document" {
			doc = string(data)
		}
	}
}

const (
	okTrue  = `{"ok":true,"result":true}`
	flood   = `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`
	flood30 = `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 30","parameters":{"retry_after":30}}`
)

var fast = Backoff{BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}

func TestCallRetries(t *testing.T) {
	reset := errors.New("connection reset by peer")
	tests := []struct {
		name        string
		policy      RetryPolicy
		timeout     time.Duration
		steps       []step
		wantCalls   int
		wantCode    int
		wantInErr   string
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{name: "server error, then success", steps: []step{{status: 502, reply: "<html>Bad Gateway</html>"}, {status: 200, reply: okTrue}}, wantCalls: 2},
		{name: "network error, then success", steps: []step{{err: reset}, {status: 200, reply: okTrue}}, wantCalls: 2},
		{name: "server error every time", steps: []step{{status: 500, reply: `{"ok":false,"error_code":500,"description":"Internal"}`}}, wantCalls: 3, wantCode: 500},
		{name: "network error every time", steps: []step{{err: reset}}, wantCalls: 3, wantInErr: "connection reset"},
		{name: "bad request is not retried", steps: []step{{status: 400, reply: `{"ok":false,"error_code":400,"description":"Bad Request"}`}}, wantCalls: 1, wantCode: 400},
		{name: "forbidden is not retried", steps: []step{{status: 403, reply: `{"ok":false,"error_code":403,"description":"Forbidden"}`}}, wantCalls: 1, wantCode: 403},
		{name: "undecodable reply is not retried", steps: []step{{status: 200, reply: "not json"}}, wantCalls: 1, wantInErr: "decoding response"},
		{name: "retries off", policy: Backoff{MaxAttempts: 1}, steps: []step{{status: 502, reply: "x"}, {status: 200, reply: okTrue}}, wantCalls: 1, wantCode: 502},
		{
			name: "flood control waits for retry_after", timeout: 5 * time.Second, steps: []step{{status: 429, reply: flood}, {status: 200, reply: okTrue}},
			wantCalls: 2, minDuration: time.Second,
		},
		{
			name: "flood control past the deadline stops at once", timeout: 2 * time.Second,
			steps: []step{{status: 429, reply: flood30}}, wantCalls: 1, wantCode: 429, maxDuration: time.Second,
		},
		{
			name: "the default timeout covers every attempt", policy: Backoff{MaxAttempts: 100, BaseDelay: 20 * time.Millisecond, MaxDelay: 40 * time.Millisecond},
			steps: []step{{status: 503, reply: "x"}}, wantCode: 503, maxDuration: 200 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The clock of the bubble is fake, so the durations are exact, and a busy machine cannot
			// delay an attempt past the deadline that the wait before it left room for.
			synctest.Test(t, func(t *testing.T) {
				policy := tt.policy
				if policy == nil {
					policy = fast
				}
				tr := &scriptedTransport{steps: tt.steps}
				c := newTestClient(t, tr, WithRetryPolicy(policy), WithDefaultTimeout(200*time.Millisecond))
				ctx := context.Background()
				if tt.timeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.timeout)
					defer cancel()
				}

				start := time.Now()
				err := c.Call(ctx, "getMe", nil, nil)
				elapsed := time.Since(start)

				var apiErr *Error
				switch {
				case tt.wantCode != 0:
					if !errors.As(err, &apiErr) || apiErr.ErrorCode != tt.wantCode {
						t.Fatalf("Call() error = %v, want an *Error with code %d", err, tt.wantCode)
					}
				case tt.wantInErr != "":
					if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
						t.Fatalf("Call() error = %v, want one mentioning %q", err, tt.wantInErr)
					}
				case err != nil:
					t.Fatalf("Call() error = %v", err)
				}
				if tt.wantCalls != 0 && tr.calls() != tt.wantCalls {
					t.Errorf("transport called %d times, want %d", tr.calls(), tt.wantCalls)
				}
				if elapsed < tt.minDuration || (tt.maxDuration > 0 && elapsed > tt.maxDuration) {
					t.Errorf("Call() took %v, want between %v and %v", elapsed, tt.minDuration, tt.maxDuration)
				}
			})
		})
	}
}

func TestCallRetriesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	cancelling := TransportFunc(func(context.Context, *Request) (*Response, error) {
		calls++
		cancel()
		return nil, errors.New("connection reset by peer")
	})
	c := newTestClient(t, cancelling, WithRetryPolicy(fast))
	if err := c.Call(ctx, "getMe", nil, nil); err == nil || calls != 1 {
		t.Fatalf("Call() = %v after %d calls, want the error after 1 call", err, calls)
	}
}

type policyFunc func(attempt int, err error) (time.Duration, bool)

func (f policyFunc) Retry(attempt int, err error) (time.Duration, bool) { return f(attempt, err) }

func TestCallRetryPolicySeesAttempts(t *testing.T) {
	var seen []string
	policy := policyFunc(func(attempt int, err error) (time.Duration, bool) {
		var apiErr *Error
		errors.As(err, &apiErr)
		seen = append(seen, strings.Repeat("|", attempt)+strings.TrimSpace(apiErr.Description))
		return 0, attempt < 2
	})
	tr := &scriptedTransport{steps: []step{{status: 500, reply: `{"ok":false,"error_code":500,"description":"first"}`},
		{status: 502, reply: `{"ok":false,"error_code":502,"description":"second"}`}}}
	c := newTestClient(t, tr, WithRetryPolicy(policy))

	err := c.Call(context.Background(), "getMe", nil, nil)

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Description != "second" {
		t.Fatalf("Call() error = %v, want the error of the last attempt", err)
	}
	if want := []string{"|first", "||second"}; strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("policy saw %q, want %q", seen, want)
	}
}

func TestCallRetriesUploads(t *testing.T) {
	reset := errors.New("connection reset by peer")
	tests := []struct {
		name      string
		file      func() models.InputFile
		first     step
		wantCalls int
		wantDocs  []string
	}{
		{
			name: "bytes are sent again", file: func() models.InputFile { return models.FileFromBytes("a", []byte("content")) },
			first: step{err: reset, readBody: true}, wantCalls: 2, wantDocs: []string{"content", "content"},
		},
		{
			name: "seekable reader is rewound", file: func() models.InputFile { return models.FileFromReader("a", strings.NewReader("content")) },
			first: step{err: reset, readBody: true}, wantCalls: 2, wantDocs: []string{"content", "content"},
		},
		{
			name: "read reader is not sent again", file: func() models.InputFile {
				return models.FileFromReader("a", io.MultiReader(strings.NewReader("content")))
			},
			first: step{err: reset, readBody: true}, wantCalls: 1, wantDocs: []string{"content"},
		},
		{
			name: "unread reader is sent after a failed connection", file: func() models.InputFile {
				return models.FileFromReader("a", io.MultiReader(strings.NewReader("content")))
			},
			first: step{err: reset}, wantCalls: 2, wantDocs: []string{"", "content"},
		},
		{
			name: "server error after an upload", file: func() models.InputFile { return models.FileFromBytes("a", []byte("content")) },
			first: step{status: 502, reply: "x", readBody: true}, wantCalls: 2, wantDocs: []string{"content", "content"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &scriptedTransport{steps: []step{tt.first, {status: 200, reply: `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`, readBody: true}}}
			c := newTestClient(t, tr, WithRetryPolicy(fast))

			_, err := c.SendDocument(context.Background(), SendDocumentParams{ChatID: models.ID(1), Document: tt.file()})

			if tt.wantCalls == 2 && err != nil {
				t.Errorf("SendDocument() error = %v", err)
			}
			if tt.wantCalls == 1 && (!errors.Is(err, reset) || errors.Is(err, upload.ErrNotReplayable)) {
				t.Errorf("SendDocument() error = %v, want the error of the attempt, %v", err, reset)
			}
			if strings.Join(tr.bodies, ",") != strings.Join(tt.wantDocs, ",") || tr.calls() != tt.wantCalls {
				t.Errorf("uploaded %q in %d calls, want %q", tr.bodies, tr.calls(), tt.wantDocs)
			}
		})
	}
}

func TestRetryAfterHeader(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"soon", 0},
		{"99999999999999", math.MaxInt64},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := retryAfterHeader(tt.in, now); got != tt.want {
				t.Errorf("retryAfterHeader(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCooldowns(t *testing.T) {
	a, b := models.ID(1), models.Username("@b")
	tests := []struct {
		name      string
		chat      models.ChatID
		deadline  time.Duration
		minWait   time.Duration
		wantRetry int // the retry_after of the error, or 0 for no error
	}{
		{name: "blocked chat waits", chat: a, minWait: 40 * time.Millisecond},
		{name: "other chat does not wait", chat: b},
		{name: "requests without a chat do not wait", chat: models.ChatID{}},
		{name: "deadline before the end fails at once", chat: a, deadline: 10 * time.Millisecond, wantRetry: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c cooldowns
			c.block(a, 50*time.Millisecond)
			c.block(a, time.Millisecond) // a shorter block does not end it early
			ctx := context.Background()
			if tt.deadline > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.deadline)
				defer cancel()
			}

			start := time.Now()
			_, err := c.wait(ctx, "sendMessage", tt.chat)
			waited := time.Since(start)

			if tt.wantRetry != 0 {
				var apiErr *Error
				if !errors.As(err, &apiErr) || !errors.Is(err, ErrTooManyRequests) || *apiErr.Parameters.RetryAfter != tt.wantRetry {
					t.Fatalf("wait() error = %v, want a 429 *Error with retry_after %d", err, tt.wantRetry)
				}
				if waited > 5*time.Millisecond {
					t.Errorf("wait() took %v before failing, want no wait", waited)
				}
				return
			}
			if err != nil || waited < tt.minWait || (tt.minWait == 0 && waited > 20*time.Millisecond) {
				t.Errorf("wait() = %v after %v, want nil after at least %v", err, waited, tt.minWait)
			}
		})
	}

	t.Run("ended blocks are forgotten", func(t *testing.T) {
		var c cooldowns
		c.block(a, time.Millisecond)
		time.Sleep(5 * time.Millisecond)
		c.block(b, time.Minute)
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.until[a]; ok || len(c.until) != 1 {
			t.Errorf("cooldowns = %v, want only %v", c.until, b)
		}
	})
}

// floodOnce answers the first request to chat 42 with flood control, and every other request with success.
type floodOnce struct {
	mu      sync.Mutex
	flooded bool
	times   []time.Time
}

func (f *floodOnce) Do(_ context.Context, req *Request) (*Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.times = append(f.times, time.Now())
	if !f.flooded && strings.Contains(string(body), `"chat_id":42`) {
		f.flooded = true
		return &Response{StatusCode: 429, Body: []byte(flood)}, nil
	}
	return &Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`)}, nil
}

func TestCallWaitsOutFloodControl(t *testing.T) {
	tr := &floodOnce{}
	c := newTestClient(t, tr)
	ctx := context.Background()
	send := func(chat models.ChatID) (time.Duration, error) {
		start := time.Now()
		_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi"})
		return time.Since(start), err
	}

	if _, err := send(models.ID(42)); !errors.Is(err, ErrTooManyRequests) {
		t.Fatalf("first SendMessage() error = %v, want flood control", err)
	}
	if took, err := send(models.ID(7)); err != nil || took > 200*time.Millisecond {
		t.Errorf("SendMessage() to another chat = %v after %v, want at once", err, took)
	}
	start := time.Now()
	if err := c.LogOut(ctx); err != nil || time.Since(start) > 200*time.Millisecond {
		t.Errorf("LogOut() without a chat = %v after %v, want at once", err, time.Since(start))
	}
	took, err := send(models.ID(42))
	if err != nil || took < 700*time.Millisecond {
		t.Errorf("SendMessage() to the flooded chat = %v after %v, want success after its retry_after", err, took)
	}
}

func TestRetryAfterFromHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		body      string
		wantRetry int
	}{
		{name: "error page", header: "3", body: "<html>Too Many Requests</html>", wantRetry: 3},
		{name: "json without parameters", header: "4", body: `{"ok":false,"error_code":429,"description":"Too Many Requests"}`, wantRetry: 4},
		{name: "json parameters win", header: "4", body: `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":9}}`, wantRetry: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", tt.header)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c := newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken))

			err := c.Call(context.Background(), "getMe", nil, nil)

			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Parameters == nil || apiErr.Parameters.RetryAfter == nil || *apiErr.Parameters.RetryAfter != tt.wantRetry {
				t.Fatalf("Call() error = %#v, want retry_after %d", err, tt.wantRetry)
			}
		})
	}
}

type waitCall struct {
	method string
	chat   models.ChatID
}

type recordingLimiter struct {
	mu    sync.Mutex
	calls []waitCall
	err   error
}

func (l *recordingLimiter) Wait(ctx context.Context, method string, chat models.ChatID) error {
	l.mu.Lock()
	l.calls = append(l.calls, waitCall{method, chat})
	l.mu.Unlock()
	if l.err == errBlock {
		<-ctx.Done()
		return ctx.Err()
	}
	return l.err
}

var errBlock = errors.New("block until the context ends")

func TestRateLimiter(t *testing.T) {
	stop := errors.New("over the budget")
	tests := []struct {
		name      string
		limitErr  error
		steps     []step
		call      func(ctx context.Context, c *Client) error
		timeout   time.Duration
		wantWaits []waitCall
		wantCalls int
		wantInErr string
		is        error
	}{
		{
			name: "chat of the request", steps: []step{{status: 200, reply: `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`}},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SendMessage(ctx, SendMessageParams{ChatID: models.ID(42), Text: "hi"})
				return err
			},
			wantWaits: []waitCall{{"sendMessage", models.ID(42)}}, wantCalls: 1,
		},
		{
			name: "username", steps: []step{{status: 200, reply: okTrue}},
			call: func(ctx context.Context, c *Client) error {
				return c.DeleteMessage(ctx, DeleteMessageParams{ChatID: models.Username("@chan"), MessageID: 1})
			},
			wantWaits: []waitCall{{"deleteMessage", models.Username("@chan")}}, wantCalls: 1,
		},
		{
			name: "no chat", steps: []step{{status: 200, reply: okTrue}},
			call:      func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			wantWaits: []waitCall{{"logOut", models.ChatID{}}}, wantCalls: 1,
		},
		{
			name: "every attempt waits", steps: []step{{status: 502, reply: "x"}, {status: 200, reply: okTrue}},
			call:      func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			wantWaits: []waitCall{{"logOut", models.ChatID{}}, {"logOut", models.ChatID{}}}, wantCalls: 2,
		},
		{
			name: "limiter error cancels the request", limitErr: stop, steps: []step{{status: 200, reply: okTrue}},
			call:      func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			wantWaits: []waitCall{{"logOut", models.ChatID{}}}, wantInErr: "logOut: rate limiter", is: stop,
		},
		{
			name: "limiter gets the context", limitErr: errBlock, timeout: 20 * time.Millisecond, steps: []step{{status: 200, reply: okTrue}},
			call:      func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			wantWaits: []waitCall{{"logOut", models.ChatID{}}}, is: context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := &recordingLimiter{err: tt.limitErr}
			tr := &scriptedTransport{steps: tt.steps}
			c := newTestClient(t, tr, WithRateLimiter(limiter), WithRetryPolicy(fast))
			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			err := tt.call(ctx, c)

			if tt.wantInErr == "" && tt.is == nil && err != nil {
				t.Fatalf("error = %v", err)
			}
			if tt.wantInErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantInErr)) {
				t.Errorf("error = %v, want one mentioning %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if !slices.Equal(limiter.calls, tt.wantWaits) {
				t.Errorf("limiter waits = %v, want %v", limiter.calls, tt.wantWaits)
			}
			if tr.calls() != tt.wantCalls {
				t.Errorf("transport called %d times, want %d", tr.calls(), tt.wantCalls)
			}
		})
	}
}

// paced calls Wait for each chat in order, each once the one before holds its turn, and returns
// how long after the start each call returned.
func paced(t *testing.T, l RateLimiter, chats []models.ChatID) []time.Duration {
	t.Helper()
	start := time.Now()
	got := make([]time.Duration, len(chats))
	var wg sync.WaitGroup
	for i, chat := range chats {
		wg.Go(func() {
			if err := l.Wait(context.Background(), "sendMessage", chat); err != nil {
				t.Errorf("Wait(%v) = %v", chat, err)
			}
			got[i] = time.Since(start)
		})
		synctest.Wait()
	}
	wg.Wait()
	return got
}

func TestNewRateLimiter(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name       string
		perSecond  int
		chatEvery  time.Duration
		groupEvery time.Duration
		chats      []models.ChatID
		want       []time.Duration
	}{
		{"rate after a burst of a second", 4, 0, 0,
			[]models.ChatID{models.ID(1), models.ID(2), models.ID(3), models.ID(4), models.ID(5), models.ID(6)}, []time.Duration{0, 0, 0, 0, 250 * ms, 500 * ms}},
		{"rate of 2", 2, 0, 0,
			[]models.ChatID{models.ID(1), models.ID(2), models.ID(3), models.ID(4), models.ID(5)}, []time.Duration{0, 0, 500 * ms, 1000 * ms, 1500 * ms}},
		{"one chat", 0, time.Second, 0,
			[]models.ChatID{models.ID(7), models.ID(7), models.ID(8), models.ID(7)}, []time.Duration{0, time.Second, 0, 2 * time.Second}},
		{"groups and channels", 0, time.Second, 3 * time.Second,
			[]models.ChatID{models.ID(-100), models.ID(-100), models.Username("@chan"), models.Username("@chan"), models.ID(7)}, []time.Duration{0, 3 * time.Second, 0, 3 * time.Second, 0}},
		{"no chat never waits", 1, time.Second, time.Second,
			[]models.ChatID{{}, {}, {}}, []time.Duration{0, 0, 0}},
		// The second request to the group waits for the group, not holding a place under the rate
		// that would keep the private chat waiting.
		{"a waiting chat holds back no other", 30, time.Second, 3 * time.Second,
			[]models.ChatID{models.ID(-100), models.ID(-100), models.ID(7)}, []time.Duration{0, 3 * time.Second, 0}},
		{"limits left out", -1, -time.Second, 0,
			[]models.ChatID{models.ID(7), models.ID(7), models.ID(-100), models.ID(-100)}, []time.Duration{0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				got := paced(t, NewRateLimiter(tt.perSecond, tt.chatEvery, tt.groupEvery), tt.chats)
				if fmt.Sprint(got) != fmt.Sprint(tt.want) {
					t.Errorf("returned after %v, want %v", got, tt.want)
				}
			})
		})
	}
}

func TestNewRateLimiterDeadline(t *testing.T) {
	tests := []struct {
		name      string
		perSecond int
		chatEvery time.Duration
	}{
		{"turn of the chat", 0, time.Second},
		{"place under the rate", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				l := NewRateLimiter(tt.perSecond, tt.chatEvery, 0)
				ctx := context.Background()
				if err := l.Wait(ctx, "sendMessage", models.ID(7)); err != nil {
					t.Fatal(err)
				}
				// The next turn is in a second: a request that must be sent sooner fails at once.
				short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				defer cancel()
				start := time.Now()
				err := l.Wait(short, "sendMessage", models.ID(7))
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 0 {
					t.Errorf("Wait() = %v after %v, want context.DeadlineExceeded at once", err, time.Since(start))
				}
				// It left its turn to the next request.
				if err := l.Wait(ctx, "sendMessage", models.ID(7)); err != nil || time.Since(start) != time.Second {
					t.Errorf("next Wait() = %v after %v, want nil after 1s", err, time.Since(start))
				}
			})
		})
	}
}

func TestNewRateLimiterCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := NewRateLimiter(0, time.Hour, 0)
		ctx, cancel := context.WithCancel(context.Background())
		if err := l.Wait(ctx, "sendMessage", models.ID(7)); err != nil {
			t.Fatal(err)
		}
		time.AfterFunc(time.Second, cancel)
		start := time.Now()
		if err := l.Wait(ctx, "sendMessage", models.ID(7)); !errors.Is(err, context.Canceled) || time.Since(start) != time.Second {
			t.Errorf("Wait() = %v after %v, want context.Canceled after 1s", err, time.Since(start))
		}
	})
}

// TestNewRateLimiterForgetsChats: a broadcast to many chats keeps only those whose turn is ahead.
func TestNewRateLimiterForgetsChats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := NewRateLimiter(0, time.Second, 0).(*paceLimiter)
		const perRound = 5000
		for round := range 20 {
			for i := range perRound {
				if err := l.Wait(context.Background(), "sendMessage", models.ID(int64(round*perRound+i+1))); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(2 * time.Second)
		}
		if n := len(l.next); n > 2*perRound+1024 {
			t.Errorf("%d chats kept after 100000, want at most %d", n, 2*perRound+1024)
		}
	})
}

// sentAt answers every request and records when each came.
type sentAt struct {
	mu    sync.Mutex
	start time.Time
	times []time.Duration
}

func (s *sentAt) Do(_ context.Context, _ *Request) (*Response, error) {
	s.mu.Lock()
	s.times = append(s.times, time.Since(s.start))
	s.mu.Unlock()
	return &Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":7,"type":"private"}}}`)}, nil
}

func TestNewRateLimiterWithClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := &sentAt{start: time.Now()}
		c := newTestClient(t, tr, WithRateLimiter(NewRateLimiter(30, time.Second, 3*time.Second)))
		for range 3 {
			if _, err := c.SendMessage(context.Background(), SendMessageParams{ChatID: models.ID(7), Text: "hi"}); err != nil {
				t.Fatal(err)
			}
		}
		if got := fmt.Sprint(tr.times); got != fmt.Sprint([]time.Duration{0, time.Second, 2 * time.Second}) {
			t.Errorf("requests sent after %s, want one a second", got)
		}
	})
}

// TestPaceLimiterSweep: sweeping forgets the chats whose turn has come and keeps the others.
func TestPaceLimiterSweep(t *testing.T) {
	l := NewRateLimiter(0, time.Second, 0).(*paceLimiter)
	now := time.Now()
	for i := range 3000 {
		at := now.Add(time.Second)
		if i%2 == 0 {
			at = now.Add(-time.Second)
		}
		l.next[models.ID(int64(i+1))] = at
	}
	l.sweep(now)
	if len(l.next) != 1500 {
		t.Fatalf("%d chats kept, want the 1500 whose turn is ahead", len(l.next))
	}
	for chat, at := range l.next {
		if !at.After(now) {
			t.Errorf("chat %v kept though its turn has come", chat)
		}
	}
}
