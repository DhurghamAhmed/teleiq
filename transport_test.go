package teleiq

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Built at run time so the source holds no token-like literal.
var (
	testSecret = strings.Repeat("Ab1_-", 7)
	testToken  = "123456789:" + testSecret
)

type trackedBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackedBody) Close() error {
	b.closed.Store(true)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

type received struct {
	method, path, contentType, body string
}

func newRecordingServer(t *testing.T, status int, reply string) (*httptest.Server, <-chan received) {
	t.Helper()
	seen := make(chan received, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- received{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func pipeBody(chunks ...string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		for _, c := range chunks {
			if _, err := io.WriteString(pw, c); err != nil {
				return
			}
		}
		_ = pw.Close()
	}()
	return pr
}

func TestHTTPTransportDo(t *testing.T) {
	big := strings.Repeat("x", 3*maxErrorBody)
	tests := []struct {
		name     string
		baseSufx string
		req      func() *Request
		status   int
		reply    string
		wantPath string
		wantType string
		wantBody string
		wantLen  int
	}{
		{
			name: "json parameters",
			req: func() *Request {
				return &Request{Method: "sendMessage", ContentType: "application/json", Body: strings.NewReader(`{"chat_id":1,"text":"hi"}`)}
			},
			status: http.StatusOK, reply: `{"ok":true,"result":{}}`,
			wantPath: "/bot" + testToken + "/sendMessage", wantType: "application/json", wantBody: `{"chat_id":1,"text":"hi"}`,
		},
		{
			name:   "no parameters",
			req:    func() *Request { return &Request{Method: "getMe", ContentType: "application/json"} },
			status: http.StatusOK, reply: `{"ok":true,"result":{"id":1}}`,
			wantPath: "/bot" + testToken + "/getMe",
		},
		{
			name:     "base url with trailing slash",
			baseSufx: "/",
			req:      func() *Request { return &Request{Method: "getMe"} },
			status:   http.StatusOK, reply: `{"ok":true,"result":{"id":1}}`,
			wantPath: "/bot" + testToken + "/getMe",
		},
		{
			name: "streamed body",
			req: func() *Request {
				return &Request{Method: "sendDocument", ContentType: "multipart/form-data; boundary=b", Body: pipeBody("part one, ", "part two")}
			},
			status: http.StatusOK, reply: `{"ok":true,"result":{}}`,
			wantPath: "/bot" + testToken + "/sendDocument", wantType: "multipart/form-data; boundary=b", wantBody: "part one, part two",
		},
		{
			name:   "error status keeps the reply",
			req:    func() *Request { return &Request{Method: "getChat"} },
			status: http.StatusBadRequest, reply: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			wantPath: "/bot" + testToken + "/getChat",
		},
		{
			name:   "error reply is capped",
			req:    func() *Request { return &Request{Method: "getMe"} },
			status: http.StatusBadGateway, reply: big,
			wantPath: "/bot" + testToken + "/getMe", wantLen: maxErrorBody,
		},
		{
			name:   "success reply is not capped",
			req:    func() *Request { return &Request{Method: "getUpdates"} },
			status: http.StatusOK, reply: big,
			wantPath: "/bot" + testToken + "/getUpdates", wantLen: len(big),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, seen := newRecordingServer(t, tt.status, tt.reply)
			tr := newHTTPTransport(srv.Client(), srv.URL+tt.baseSufx, testToken)

			resp, err := tr.Do(context.Background(), tt.req())
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			got := <-seen
			if got.method != http.MethodPost {
				t.Errorf("HTTP method = %q, want POST", got.method)
			}
			if got.path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.path, tt.wantPath)
			}
			if got.contentType != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got.contentType, tt.wantType)
			}
			if got.body != tt.wantBody {
				t.Errorf("request body = %q, want %q", got.body, tt.wantBody)
			}
			if resp.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tt.status)
			}
			wantLen := tt.wantLen
			if wantLen == 0 {
				wantLen = len(tt.reply)
			}
			if len(resp.Body) != wantLen || string(resp.Body) != tt.reply[:wantLen] {
				t.Errorf("reply body has %d bytes, want the first %d of the reply", len(resp.Body), wantLen)
			}
		})
	}
}

func TestHTTPTransportErrors(t *testing.T) {
	// The server only notices the client leaving once the body is read.
	blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	t.Cleanup(blocking.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	var hits atomic.Int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(counting.Close)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		baseURL string
		ctx     func() (context.Context, context.CancelFunc)
		method  string
		is      error
		urlErr  bool
	}{
		{name: "server unreachable", baseURL: closedURL, method: "getMe", urlErr: true},
		{name: "canceled context", baseURL: counting.URL, method: "getMe", is: context.Canceled, urlErr: true,
			ctx: func() (context.Context, context.CancelFunc) { return canceled, func() {} }},
		{name: "deadline exceeded", baseURL: blocking.URL, method: "getMe", is: context.DeadlineExceeded, urlErr: true,
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			}},
		{name: "empty method", baseURL: counting.URL, method: ""},
		{name: "method with slash", baseURL: counting.URL, method: "get/Me"},
		{name: "method with dots", baseURL: counting.URL, method: "../getMe"},
		{name: "method with query", baseURL: counting.URL, method: "getMe?x=1"},
		{name: "method with token", baseURL: counting.URL, method: testToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.Background(), context.CancelFunc(func() {})
			if tt.ctx != nil {
				ctx, cancel = tt.ctx()
			}
			defer cancel()
			before := hits.Load()
			body := &trackedBody{Reader: strings.NewReader("{}")}
			tr := newHTTPTransport(&http.Client{}, tt.baseURL, testToken)

			resp, err := tr.Do(ctx, &Request{Method: tt.method, ContentType: "application/json", Body: body})
			if err == nil {
				t.Fatalf("Do() = %+v, want an error", resp)
			}
			for e := error(err); e != nil; e = errors.Unwrap(e) {
				if strings.Contains(e.Error(), testSecret) {
					t.Fatalf("error leaks the token: %q", e.Error())
				}
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			if tt.urlErr {
				var ue *url.Error
				if !errors.As(err, &ue) {
					t.Fatalf("errors.As(%v, *url.Error) = false, want true", err)
				}
				if strings.Contains(ue.URL, testSecret) {
					t.Errorf("url.Error.URL leaks the token: %q", ue.URL)
				}
			}
			if !body.closed.Load() {
				t.Error("request body was not closed")
			}
			if tt.baseURL == counting.URL && tt.is == nil && hits.Load() != before {
				t.Error("an invalid method reached the server")
			}
		})
	}
}

func TestHTTPTransportClosesResponseBody(t *testing.T) {
	tests := []struct {
		name     string
		download io.Writer // downloads to it instead of calling a method, if set
		status   int
		reader   io.Reader
		wantErr  bool
	}{
		{name: "success", status: http.StatusOK, reader: strings.NewReader(`{"ok":true}`)},
		{name: "error status", status: http.StatusInternalServerError, reader: strings.NewReader(`{"ok":false}`)},
		{name: "flood control", status: http.StatusTooManyRequests, reader: strings.NewReader(`{"ok":false}`)},
		{name: "large error page", status: http.StatusBadGateway, reader: strings.NewReader(strings.Repeat("x", 2*maxErrorBody))},
		{name: "read failure", status: http.StatusOK, reader: failingReader{}, wantErr: true},
		{name: "download", download: io.Discard, status: http.StatusOK, reader: strings.NewReader("content")},
		{name: "download not found", download: io.Discard, status: http.StatusNotFound, reader: strings.NewReader(`{"ok":false}`), wantErr: true},
		{name: "download read failure", download: io.Discard, status: http.StatusOK, reader: failingReader{}, wantErr: true},
		{name: "download write failure", download: failingWriter{}, status: http.StatusOK, reader: strings.NewReader("content"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: tt.reader}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: body, Request: r}, nil
			})}
			tr := newHTTPTransport(client, "https://bot.invalid", testToken)

			var err error
			if tt.download != nil {
				err = tr.download(context.Background(), "documents/a.txt", tt.download)
			} else {
				_, err = tr.Do(context.Background(), &Request{Method: "getMe"})
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), testSecret) {
				t.Errorf("error leaks the token: %q", err.Error())
			}
			if !body.closed.Load() {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestHTTPTransportConcurrentUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	t.Cleanup(srv.Close)
	tr := newHTTPTransport(srv.Client(), srv.URL, testToken)

	var wg sync.WaitGroup
	errs := make(chan error, 16*10)
	for range 16 {
		wg.Go(func() {
			for range 10 {
				if _, err := tr.Do(context.Background(), &Request{Method: "getMe"}); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Do() error = %v", err)
	}
}

func TestTransportFunc(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	want := &Response{StatusCode: http.StatusOK, Body: []byte(`{"ok":true}`)}
	var gotCtx context.Context
	var gotReq *Request
	var tr Transport = TransportFunc(func(ctx context.Context, req *Request) (*Response, error) {
		gotCtx, gotReq = ctx, req
		return want, nil
	})

	req := &Request{Method: "getMe"}
	resp, err := tr.Do(ctx, req)
	if err != nil || resp != want {
		t.Fatalf("Do() = %v, %v; want %v, nil", resp, err, want)
	}
	if gotCtx != ctx || gotReq != req {
		t.Error("TransportFunc did not pass its arguments through")
	}
}

func TestNewDefaultHTTPClient(t *testing.T) {
	c := newDefaultHTTPClient()
	if c == http.DefaultClient {
		t.Fatal("default client is http.DefaultClient")
	}
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0: deadlines come from the request context", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if http.RoundTripper(tr) == http.DefaultTransport {
		t.Error("default client shares http.DefaultTransport")
	}
	if tr.MaxIdleConnsPerHost < 8 {
		t.Errorf("MaxIdleConnsPerHost = %d, want enough to keep concurrent requests pooled", tr.MaxIdleConnsPerHost)
	}
	if tr.Proxy == nil {
		t.Error("Proxy is nil, want proxy settings from the environment")
	}
	if tr.DisableKeepAlives {
		t.Error("keep-alive is disabled")
	}
}
