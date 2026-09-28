package teleiq

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

type testUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
}

type recordedCall struct {
	calls       atomic.Int32
	ctx         context.Context
	req         *Request
	body        string
	deadline    time.Time
	hasDeadline bool
}

func fakeTransport(status int, reply string, rec *recordedCall) Transport {
	return TransportFunc(func(ctx context.Context, req *Request) (*Response, error) {
		if rec != nil {
			rec.calls.Add(1)
			rec.ctx, rec.req = ctx, req
			rec.deadline, rec.hasDeadline = ctx.Deadline()
			if req.Body != nil {
				b, _ := io.ReadAll(req.Body)
				rec.body = string(b)
			}
		}
		return &Response{StatusCode: status, Body: []byte(reply)}, nil
	})
}

func newTestClient(t testing.TB, tr Transport, opts ...Option) *Client {
	t.Helper()
	// Retries are off unless a test sets its own policy, so that failures are not repeated.
	c, err := NewClient(testToken, append([]Option{WithTransport(tr), WithRetryPolicy(Backoff{MaxAttempts: 1})}, opts...)...)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

func TestNewClientToken(t *testing.T) {
	tests := []struct {
		token string
		valid bool
	}{
		{testToken, true},
		{"1:a", true},
		{"123456:ABC-def_ghi", true},
		{"", false},
		{"123", false},
		{"123:", false},
		{":abc", false},
		{"12a:abc", false},
		{"123:abc def", false},
		{"123:abc/def", false},
		{" 123:abc", false},
		{"123:abc:def", false},
		{"123:abc\n", false},
		{"123:аbc", false},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			c, err := NewClient(tt.token)
			if tt.valid {
				if err != nil || c == nil {
					t.Fatalf("NewClient() = %v, %v; want a client", c, err)
				}
				return
			}
			if c != nil || !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("NewClient() = %v, %v; want nil, ErrInvalidToken", c, err)
			}
			if tt.token != "" && strings.Contains(err.Error(), tt.token) {
				t.Errorf("error %q repeats the token", err)
			}
		})
	}
}

func TestNewClientOptions(t *testing.T) {
	custom := TransportFunc(func(context.Context, *Request) (*Response, error) { return nil, nil })
	rt := http.DefaultTransport
	hc := &http.Client{Timeout: 7 * time.Second, Transport: rt}

	tests := []struct {
		name        string
		opts        []Option
		wantErr     bool
		wantTimeout time.Duration
		check       func(t *testing.T, c *Client)
	}{
		{
			name: "defaults", wantTimeout: 30 * time.Second,
			check: func(t *testing.T, c *Client) {
				ht, ok := c.transport.(*httpTransport)
				if !ok {
					t.Fatalf("transport is %T, want *httpTransport", c.transport)
				}
				if ht.baseURL != "https://api.telegram.org" || ht.token != testToken {
					t.Errorf("transport targets %q with token set %v", ht.baseURL, ht.token == testToken)
				}
				if ht.client == http.DefaultClient || ht.client.Timeout != 0 {
					t.Errorf("default HTTP client is shared or has a client-wide timeout")
				}
				if c.retry != RetryPolicy(Backoff{}) {
					t.Errorf("default retry policy = %#v, want Backoff{}", c.retry)
				}
			},
		},
		{
			name: "http client used as is", opts: []Option{WithHTTPClient(hc)}, wantTimeout: 30 * time.Second,
			check: func(t *testing.T, c *Client) {
				if ht := c.transport.(*httpTransport); ht.client != hc {
					t.Error("transport does not use the given HTTP client")
				}
				if hc.Timeout != 7*time.Second || hc.Transport != rt {
					t.Error("the given HTTP client was modified")
				}
			},
		},
		{
			name: "custom transport", opts: []Option{WithTransport(custom)}, wantTimeout: 30 * time.Second,
			check: func(t *testing.T, c *Client) {
				if _, ok := c.transport.(TransportFunc); !ok {
					t.Errorf("transport is %T, want the given TransportFunc", c.transport)
				}
			},
		},
		{name: "custom default timeout", opts: []Option{WithDefaultTimeout(5 * time.Second)}, wantTimeout: 5 * time.Second},
		{
			name: "custom retry policy", opts: []Option{WithRetryPolicy(Backoff{MaxAttempts: 5})}, wantTimeout: 30 * time.Second,
			check: func(t *testing.T, c *Client) {
				if c.retry != RetryPolicy(Backoff{MaxAttempts: 5}) {
					t.Errorf("retry policy = %#v, want the given one", c.retry)
				}
			},
		},
		{name: "nil retry policy", opts: []Option{WithRetryPolicy(nil)}, wantErr: true},
		{name: "nil rate limiter", opts: []Option{WithRateLimiter(nil)}, wantErr: true},
		{name: "nil logger", opts: []Option{WithLogger(nil)}, wantErr: true},
		{name: "default timeout disabled", opts: []Option{WithDefaultTimeout(0)}, wantTimeout: 0},
		{name: "nil option ignored", opts: []Option{nil}, wantTimeout: 30 * time.Second},
		{name: "negative timeout", opts: []Option{WithDefaultTimeout(-time.Second)}, wantErr: true},
		{name: "nil http client", opts: []Option{WithHTTPClient(nil)}, wantErr: true},
		{name: "nil transport", opts: []Option{WithTransport(nil)}, wantErr: true},
		{name: "http client with transport", opts: []Option{WithHTTPClient(hc), WithTransport(custom)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(testToken, tt.opts...)
			if tt.wantErr {
				if err == nil || errors.Is(err, ErrInvalidToken) {
					t.Fatalf("NewClient() error = %v, want an option error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if c.defaultTimeout != tt.wantTimeout {
				t.Errorf("default timeout = %v, want %v", c.defaultTimeout, tt.wantTimeout)
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

func TestClientFormatHidesToken(t *testing.T) {
	c, err := NewClient(testToken)
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("client", "c", c, "value", *c)
	for _, s := range []string{fmt.Sprintf("%v %+v %#v", c, c, c), fmt.Sprintf("%v %+v %#v", *c, *c, *c), logged.String()} {
		if strings.Contains(s, testSecret) {
			t.Errorf("formatting the Client shows the token: %s", s)
		}
	}
}

func TestWithBaseURL(t *testing.T) {
	fake := TransportFunc(func(context.Context, *Request) (*Response, error) { return nil, nil })
	tests := []struct {
		name      string
		opts      []Option
		want      string
		wantInErr string
	}{
		{name: "default", want: "https://api.telegram.org"},
		{name: "local server", opts: []Option{WithBaseURL("http://localhost:8081")}, want: "http://localhost:8081"},
		{name: "path behind a proxy", opts: []Option{WithBaseURL("https://proxy.example.com/telegram/")}, want: "https://proxy.example.com/telegram"},
		{name: "empty", opts: []Option{WithBaseURL("")}, wantInErr: "not an http or https URL"},
		{name: "no scheme", opts: []Option{WithBaseURL("localhost:8081")}, wantInErr: "not an http or https URL"},
		{name: "other scheme", opts: []Option{WithBaseURL("ftp://example.com")}, wantInErr: "not an http or https URL"},
		{name: "no host", opts: []Option{WithBaseURL("http://")}, wantInErr: "has no host"},
		{name: "credentials", opts: []Option{WithBaseURL("https://user:secret@example.com")}, wantInErr: "cannot hold credentials"},
		{name: "query", opts: []Option{WithBaseURL("https://example.com/?a=1")}, wantInErr: "query or a fragment"},
		{name: "fragment", opts: []Option{WithBaseURL("https://example.com/#top")}, wantInErr: "query or a fragment"},
		{name: "with a transport", opts: []Option{WithBaseURL("http://localhost:8081"), WithTransport(fake)}, wantInErr: "cannot be combined"},
		{name: "token without a scheme", opts: []Option{WithBaseURL("api.telegram.org/bot" + testToken)}, wantInErr: "not an http or https URL"},
		{name: "token in a query", opts: []Option{WithBaseURL("https://api.telegram.org/?token=" + testToken)}, wantInErr: "query or a fragment"},
		{name: "token in an invalid URL", opts: []Option{WithBaseURL("https://api.telegram.org/bot" + testToken + "/%zz")}, wantInErr: "invalid URL escape"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(testToken, tt.opts...)
			if tt.wantInErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("NewClient() error = %v, want one mentioning %q", err, tt.wantInErr)
				}
				if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), testSecret) {
					t.Errorf("error %q repeats the credentials", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if got := c.transport.(*httpTransport).baseURL; got != tt.want {
				t.Errorf("base URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseURLRequests(t *testing.T) {
	for _, prefix := range []string{"", "/telegram"} {
		t.Run("prefix "+prefix, func(t *testing.T) {
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, strings.ReplaceAll(r.URL.Path, testToken, "<token>"))
				switch r.URL.Path {
				case prefix + "/bot" + testToken + "/getMe":
					_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Local"}}`)
				case prefix + "/file/bot" + testToken + "/videos/big.mp4":
					_, _ = io.WriteString(w, "video")
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			c, err := NewClient(testToken, WithBaseURL(srv.URL+prefix+"/"), WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}

			me, err := c.GetMe(context.Background())
			if err != nil || me.FirstName != "Local" {
				t.Fatalf("GetMe() = %+v, %v", me, err)
			}
			var video bytes.Buffer
			if err := c.Download(context.Background(), "videos/big.mp4", &video); err != nil || video.String() != "video" {
				t.Fatalf("Download() = %q, %v", video.String(), err)
			}
			want := []string{prefix + "/bot<token>/getMe", prefix + "/file/bot<token>/videos/big.mp4"}
			if strings.Join(paths, ",") != strings.Join(want, ",") {
				t.Errorf("server saw %v, want %v", paths, want)
			}
		})
	}
}

const getMeReply = `{"ok":true,"result":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"test_bot",` +
	`"can_join_groups":true,"can_read_all_group_messages":false,"supports_inline_queries":false,"future_field":1}}`

var getMeUser = &models.User{
	ID: 123456789, IsBot: true, FirstName: "Test Bot", Username: Ptr("test_bot"),
	CanJoinGroups: Ptr(true), CanReadAllGroupMessages: Ptr(false), SupportsInlineQueries: Ptr(false),
}

func TestGetMe(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		reply     string
		want      *models.User
		is        error
		wantInErr string
	}{
		{name: "bot user", status: 200, reply: getMeReply, want: getMeUser},
		{name: "minimal user", status: 200, reply: `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"B"}}`, want: &models.User{ID: 1, IsBot: true, FirstName: "B"}},
		{name: "rejected token", status: 401, reply: `{"ok":false,"error_code":401,"description":"Unauthorized"}`, is: ErrUnauthorized, wantInErr: "getMe"},
		{name: "result of the wrong shape", status: 200, reply: `{"ok":true,"result":[1]}`, wantInErr: "decoding result"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(tt.status, tt.reply, rec))

			got, err := c.GetMe(context.Background())

			if rec.req.Method != "getMe" || rec.req.Body != nil || rec.req.ContentType != "" {
				t.Errorf("request = %q with body %v, want getMe without parameters", rec.req.Method, rec.req.Body != nil)
			}
			if tt.want == nil {
				if got != nil || err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("GetMe() = %+v, %v; want nil and an error mentioning %q", got, err, tt.wantInErr)
				}
				if tt.is != nil && !errors.Is(err, tt.is) {
					t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("GetMe() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestGetMeThroughHTTPTransport(t *testing.T) {
	srv, seen := newRecordingServer(t, http.StatusOK, getMeReply)
	c := newTestClient(t, newHTTPTransport(srv.Client(), srv.URL, testToken))

	got, err := c.GetMe(context.Background())
	if err != nil || !reflect.DeepEqual(got, getMeUser) {
		t.Fatalf("GetMe() = %+v, %v; want %+v", got, err, getMeUser)
	}
	r := <-seen
	if r.method != http.MethodPost || r.path != "/bot"+testToken+"/getMe" || r.body != "" {
		t.Errorf("server saw %s %s with body %q, want POST /bot<token>/getMe without a body", r.method, strings.ReplaceAll(r.path, testToken, "<token>"), r.body)
	}
}

// TestTimeoutPrecedence checks the rules of every timeout against a server that answers after 200ms.
func TestTimeoutPrecedence(t *testing.T) {
	const slow, short, long = 200 * time.Millisecond, 50 * time.Millisecond, 5 * time.Second
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-time.After(slow):
		case <-r.Context().Done():
			return
		}
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "file")
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"id":1,"is_bot":true,"first_name":"B"}}`)
	}))
	t.Cleanup(srv.Close)

	getMe := func(ctx context.Context, c *Client) error { _, err := c.GetMe(ctx); return err }
	upload := func(ctx context.Context, c *Client) error {
		_, err := c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(1), Document: models.FileFromBytes("a.txt", []byte("x"))})
		return err
	}
	byFileID := func(ctx context.Context, c *Client) error {
		_, err := c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(1), Document: models.FileID("AgAD")})
		return err
	}
	download := func(ctx context.Context, c *Client) error { return c.Download(ctx, "a.txt", io.Discard) }

	tests := []struct {
		name        string
		defaultTo   time.Duration
		deadline    time.Duration // of the caller's context, if any
		httpTimeout time.Duration // of a client given to WithHTTPClient, if any
		call        func(context.Context, *Client) error
		wantTimeout bool
	}{
		{name: "default timeout ends a slow call", defaultTo: short, call: getMe, wantTimeout: true},
		{name: "longer caller deadline wins", defaultTo: short, deadline: long, call: getMe},
		{name: "shorter caller deadline wins", defaultTo: long, deadline: short, call: getMe, wantTimeout: true},
		{name: "no default timeout", defaultTo: 0, call: getMe},
		{name: "uploads have no default timeout", defaultTo: short, call: upload},
		{name: "uploads keep the caller deadline", defaultTo: long, deadline: short, call: upload, wantTimeout: true},
		{name: "a file ID is not an upload", defaultTo: short, call: byFileID, wantTimeout: true},
		{name: "downloads have no default timeout", defaultTo: short, call: download},
		{name: "downloads keep the caller deadline", defaultTo: long, deadline: short, call: download, wantTimeout: true},
		{name: "the timeout of a given client still applies", defaultTo: long, httpTimeout: short, call: getMe, wantTimeout: true},
		{name: "even to uploads", defaultTo: long, httpTimeout: short, call: upload, wantTimeout: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := srv.Client()
			if tt.httpTimeout > 0 {
				hc = &http.Client{Transport: srv.Client().Transport, Timeout: tt.httpTimeout}
			}
			c, err := NewClient(testToken, WithBaseURL(srv.URL), WithHTTPClient(hc),
				WithDefaultTimeout(tt.defaultTo), WithRetryPolicy(Backoff{MaxAttempts: 1}))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if tt.deadline > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.deadline)
				defer cancel()
			}

			start := time.Now()
			err = tt.call(ctx, c)
			elapsed := time.Since(start)

			var netErr net.Error
			timedOut := errors.As(err, &netErr) && netErr.Timeout()
			if timedOut != tt.wantTimeout || (!tt.wantTimeout && err != nil) {
				t.Fatalf("error = %v after %v, want a timeout = %v", err, elapsed, tt.wantTimeout)
			}
			if tt.wantTimeout && elapsed >= slow {
				t.Errorf("timed out after %v, want before the %v reply", elapsed, slow)
			}
		})
	}
}

func TestCallDefaultTimeout(t *testing.T) {
	longer := time.Now().Add(2 * time.Hour)
	shorter := time.Now().Add(time.Second)
	tests := []struct {
		name         string
		opts         []Option
		deadline     time.Time
		wantDeadline bool
		wantAfter    time.Duration
		wantExact    time.Time
	}{
		{name: "default applies without a deadline", wantDeadline: true, wantAfter: 30 * time.Second},
		{name: "custom default", opts: []Option{WithDefaultTimeout(5 * time.Second)}, wantDeadline: true, wantAfter: 5 * time.Second},
		{name: "default disabled", opts: []Option{WithDefaultTimeout(0)}},
		{name: "longer caller deadline wins", deadline: longer, wantDeadline: true, wantExact: longer},
		{name: "shorter caller deadline wins", deadline: shorter, wantDeadline: true, wantExact: shorter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordedCall{}
			c := newTestClient(t, fakeTransport(200, `{"ok":true,"result":true}`, rec), tt.opts...)
			ctx := context.Background()
			if !tt.deadline.IsZero() {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, tt.deadline)
				defer cancel()
			}

			start := time.Now()
			if err := c.Call(ctx, "getMe", nil, nil); err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if rec.hasDeadline != tt.wantDeadline {
				t.Fatalf("transport context has deadline = %v, want %v", rec.hasDeadline, tt.wantDeadline)
			}
			switch {
			case !tt.wantExact.IsZero():
				if !rec.deadline.Equal(tt.wantExact) {
					t.Errorf("deadline = %v, want the caller's %v", rec.deadline, tt.wantExact)
				}
			case tt.wantAfter > 0:
				if d := rec.deadline.Sub(start); d < tt.wantAfter-time.Second || d > tt.wantAfter+time.Second {
					t.Errorf("deadline is %v after the call, want about %v", d, tt.wantAfter)
				}
				if rec.ctx.Err() == nil {
					t.Error("the default-timeout context was not released after Call returned")
				}
			}
		})
	}
}

func TestCallDefaultTimeoutCancelsStuckRequest(t *testing.T) {
	stuck := TransportFunc(func(ctx context.Context, _ *Request) (*Response, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return nil, errors.New("default timeout never fired")
		}
	})
	c := newTestClient(t, stuck, WithDefaultTimeout(50*time.Millisecond))

	start := time.Now()
	err := c.Call(context.Background(), "getMe", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Call() took %v, want it cut off by the 50ms default timeout", elapsed)
	}
}

// TestIntegrationFiles uploads and downloads files through the real Bot API when
// TELEIQ_TEST_TOKEN and TELEIQ_TEST_CHAT_ID are set; it is skipped otherwise.
func TestIntegrationFiles(t *testing.T) {
	token, chat := os.Getenv("TELEIQ_TEST_TOKEN"), os.Getenv("TELEIQ_TEST_CHAT_ID")
	if token == "" || chat == "" {
		t.Skip("set TELEIQ_TEST_TOKEN and TELEIQ_TEST_CHAT_ID to run against the Bot API")
	}
	chatID, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		t.Fatalf("TELEIQ_TEST_CHAT_ID: %v", err)
	}
	c, err := NewClient(token)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	content := []byte("Uploaded by the teleiq integration test.\n")
	msg, err := c.SendDocument(ctx, SendDocumentParams{
		ChatID:   models.ID(chatID),
		Document: models.FileFromBytes("teleiq-upload.txt", content),
		Caption:  Ptr("teleiq: upload"),
	})
	if err != nil {
		t.Fatalf("SendDocument(upload) error = %v", err)
	}
	if msg.Document == nil || msg.Document.FileName == nil || *msg.Document.FileName != "teleiq-upload.txt" {
		t.Fatalf("SendDocument(upload) document = %+v", msg.Document)
	}

	album, err := c.SendMediaGroup(ctx, SendMediaGroupParams{ChatID: models.ID(chatID), Media: []models.InputMediaGroupItem{
		&models.InputMediaDocument{Media: models.FileFromBytes("teleiq-1.txt", []byte("first\n"))},
		&models.InputMediaDocument{Media: models.FileFromBytes("teleiq-2.txt", []byte("second\n")), Caption: Ptr("teleiq: album")},
	}})
	if err != nil || len(album) != 2 || album[0].Document == nil || album[1].Document == nil {
		t.Fatalf("SendMediaGroup(uploads) = %d messages, %v", len(album), err)
	}

	if _, err := c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(chatID), Document: models.FileID(msg.Document.FileID)}); err != nil {
		t.Fatalf("SendDocument(file ID) error = %v", err)
	}

	info, err := c.GetFile(ctx, GetFileParams{FileID: msg.Document.FileID})
	if err != nil || info.FilePath == nil {
		t.Fatalf("GetFile() = %+v, %v", info, err)
	}
	var downloaded bytes.Buffer
	if err := c.Download(ctx, *info.FilePath, &downloaded); err != nil || !bytes.Equal(downloaded.Bytes(), content) {
		t.Fatalf("Download() = %q, %v; want %q", downloaded.Bytes(), err, content)
	}
}

// TestREADMENamesTheBotAPIVersion checks that the README, its badge included, names the Bot API
// version that the generated code implements, so a new release cannot leave it behind.
func TestREADMENamesTheBotAPIVersion(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`Bot(?: |%20)API[ -](\d+\.\d+)`).FindAllSubmatch(readme, -1)
	if len(found) == 0 {
		t.Fatal("README.md does not name the Bot API version")
	}
	for _, m := range found {
		if string(m[1]) != SupportedBotAPIVersion {
			t.Errorf("README.md names Bot API %s, want %s", m[1], SupportedBotAPIVersion)
		}
	}
}

// openedFiles counts the readers that uploads open, to check that each is closed.
type openedFiles struct {
	mu    sync.Mutex
	files []*trackedBody
}

func (o *openedFiles) file(name string) models.InputFile {
	return models.FileFromReopener(name, func() (io.ReadCloser, error) {
		o.mu.Lock()
		defer o.mu.Unlock()
		f := &trackedBody{Reader: strings.NewReader("file content")}
		o.files = append(o.files, f)
		return f, nil
	})
}

func (o *openedFiles) open() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, f := range o.files {
		if !f.closed.Load() {
			n++
		}
	}
	return n
}

type waitingLimiter struct{}

func (waitingLimiter) Wait(ctx context.Context, _ string, _ models.ChatID) error {
	<-ctx.Done()
	return ctx.Err()
}

// TestCallLeavesNothingBehind runs calls in synctest bubbles, whose clock is fake: when a test
// function returns, synctest fails it if a goroutine it started is still blocked, even on a timer.
// Each call must also return as soon as its context ends and close every file it opened.
func TestCallLeavesNothingBehind(t *testing.T) {
	tests := []struct {
		name    string
		reply   func(ctx context.Context, call int, req *Request) (*Response, error)
		upload  bool
		cancel  time.Duration // when the context of the call is canceled, if at all
		opts    []Option
		before  func(c *Client) // runs before the call
		wantErr bool
		took    time.Duration
	}{
		{name: "retried until it succeeds", took: 2 * time.Second, reply: func(_ context.Context, call int, _ *Request) (*Response, error) {
			if call < 3 {
				return &Response{StatusCode: 502, Body: []byte("Bad Gateway")}, nil
			}
			return &Response{StatusCode: 200, Body: []byte(sentMessageReply)}, nil
		}},
		{name: "canceled while waiting to retry", cancel: 100 * time.Millisecond, wantErr: true, took: 100 * time.Millisecond,
			reply: func(context.Context, int, *Request) (*Response, error) {
				return &Response{StatusCode: 502, Body: []byte("Bad Gateway")}, nil
			}},
		{name: "canceled while waiting to retry after flood control", cancel: time.Second, wantErr: true, took: time.Second,
			reply: func(context.Context, int, *Request) (*Response, error) {
				return &Response{StatusCode: 429, Body: []byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":5}}`)}, nil
			}},
		{name: "canceled in the flood control of the chat", cancel: time.Second, wantErr: true, took: time.Second,
			before: func(c *Client) {
				_, _ = c.SendMessage(context.Background(), SendMessageParams{ChatID: models.ID(1), Text: "hi"})
			},
			reply: func(context.Context, int, *Request) (*Response, error) {
				return &Response{StatusCode: 429, Body: []byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":5}}`)}, nil
			}},
		{name: "canceled in the rate limiter", cancel: time.Second, wantErr: true, took: time.Second, opts: []Option{WithRateLimiter(waitingLimiter{})},
			reply: func(context.Context, int, *Request) (*Response, error) { return nil, errors.New("not called") }},
		{name: "upload retried after a partial read", upload: true, took: 2 * time.Second,
			reply: func(_ context.Context, call int, req *Request) (*Response, error) {
				if call == 1 {
					_, _ = io.ReadFull(req.Body, make([]byte, 3))
					return nil, errors.New("connection reset by peer")
				}
				_, _ = io.Copy(io.Discard, req.Body)
				return &Response{StatusCode: 200, Body: []byte(sentMessageReply)}, nil
			}},
		{name: "upload never read", upload: true, wantErr: true, took: 20 * time.Second,
			reply: func(context.Context, int, *Request) (*Response, error) {
				return nil, errors.New("connection refused")
			}},
		{name: "upload canceled while sending", upload: true, cancel: 250 * time.Millisecond, wantErr: true, took: 250 * time.Millisecond,
			reply: func(ctx context.Context, _ int, req *Request) (*Response, error) {
				for {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(100 * time.Millisecond):
					}
					if _, err := req.Body.Read(make([]byte, 1)); err != nil {
						return nil, err
					}
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var mu sync.Mutex
				calls := 0
				tr := TransportFunc(func(ctx context.Context, req *Request) (*Response, error) {
					mu.Lock()
					calls++
					call := calls
					mu.Unlock()
					return tt.reply(ctx, call, req)
				})
				c := newTestClient(t, tr, append([]Option{WithRetryPolicy(Backoff{})}, tt.opts...)...)
				if tt.before != nil {
					tt.before(c)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tt.cancel > 0 {
					time.AfterFunc(tt.cancel, cancel)
				}
				var files openedFiles
				start := time.Now()
				var err error
				if tt.upload {
					_, err = c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(1), Document: files.file("a.txt")})
				} else {
					_, err = c.SendMessage(ctx, SendMessageParams{ChatID: models.ID(1), Text: "hi"})
				}
				if took := time.Since(start); (err != nil) != tt.wantErr || took > tt.took {
					t.Errorf("call = %v after %v, want error %v within %v", err, took, tt.wantErr, tt.took)
				}
				if tt.cancel == 0 {
					if n := files.open(); n > 0 {
						t.Errorf("%d of the %d opened files are still open when the call returns", n, len(files.files))
					}
				}
				synctest.Wait()
				if n := files.open(); n > 0 {
					t.Errorf("%d of the %d opened files are still open", n, len(files.files))
				}
			})
		})
	}
}

func TestUploadClosesFilesWhenAnotherFails(t *testing.T) {
	var files openedFiles
	c := newTestClient(t, fakeTransport(200, sentMessageReply, nil))
	_, err := c.SendVideo(context.Background(), SendVideoParams{ChatID: models.ID(1), Video: files.file("v.mp4"),
		Thumbnail: models.FileFromReopener("t.jpg", func() (io.ReadCloser, error) { return nil, os.ErrNotExist })})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SendVideo() = %v, want the error of the thumbnail", err)
	}
	if n := files.open(); n != 0 || len(files.files) != 1 {
		t.Errorf("%d of the %d opened files are still open", n, len(files.files))
	}
}

// TestFileFromPathIsClosed counts the open file descriptors of the process around uploads.
func TestFileFromPathIsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("counts the entries of /dev/fd")
	}
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("file content"), 0o600); err != nil {
		t.Fatal(err)
	}
	openFiles := func() int {
		entries, err := os.ReadDir("/dev/fd")
		if err != nil {
			t.Skip(err)
		}
		return len(entries)
	}
	replies := []Transport{
		fakeTransport(200, sentMessageReply, &recordedCall{}),
		TransportFunc(func(context.Context, *Request) (*Response, error) { return nil, errors.New("connection refused") }),
	}
	before := openFiles()
	for _, tr := range replies {
		c := newTestClient(t, tr, WithRetryPolicy(Backoff{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}))
		_, _ = c.SendDocument(context.Background(), SendDocumentParams{ChatID: models.ID(1), Document: models.FileFromPath(path)})
	}
	if after := openFiles(); after > before {
		t.Errorf("%d files open after the uploads, %d before", after, before)
	}
}

// blockingReader blocks every Read until release is closed.
type blockingReader struct{ release chan struct{} }

func (r blockingReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

// TestCanceledUploadDoesNotWaitForItsReader checks that a call returns when its context ends even
// while the upload is blocked reading the caller's reader.
func TestCanceledUploadDoesNotWaitForItsReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := blockingReader{release: make(chan struct{})}
		c := newTestClient(t, TransportFunc(func(ctx context.Context, req *Request) (*Response, error) {
			go func() { _, _ = io.Copy(io.Discard, req.Body) }()
			<-ctx.Done()
			return nil, ctx.Err()
		}))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		_, err := c.SendDocument(ctx, SendDocumentParams{ChatID: models.ID(1), Document: models.FileFromReader("a.txt", r)})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
			t.Errorf("SendDocument() = %v after %v, want the deadline after 1s", err, time.Since(start))
		}
		close(r.release)
	})
}
