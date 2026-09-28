package teleiq

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

// Transport sends one Bot API request and returns the raw reply.
type Transport interface {
	Do(ctx context.Context, req *Request) (*Response, error)
}

// TransportFunc adapts a function to the Transport interface.
type TransportFunc func(ctx context.Context, req *Request) (*Response, error)

// Do calls f(ctx, req).
func (f TransportFunc) Do(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

// Request is one Bot API call: the method name and its encoded parameters.
type Request struct {
	Method      string
	ContentType string
	Body        io.Reader
}

// Response is the HTTP status and raw body of a Bot API reply.
type Response struct {
	StatusCode int
	Body       []byte
	RetryAfter time.Duration // the Retry-After header of a 429 reply, if any
}

const maxErrorBody = 64 << 10

type httpTransport struct {
	client  *http.Client
	baseURL string
	token   string
}

func newHTTPTransport(client *http.Client, baseURL, token string) *httpTransport {
	return &httpTransport{client: client, baseURL: strings.TrimRight(baseURL, "/"), token: token}
}

func (t *httpTransport) Do(ctx context.Context, req *Request) (*Response, error) {
	if !validMethod(req.Method) {
		closeBody(req.Body)
		return nil, sanitize.Error(fmt.Errorf("teleiq: invalid method name %q", req.Method))
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/bot"+t.token+"/"+req.Method, req.Body)
	if err != nil {
		closeBody(req.Body)
		return nil, sanitize.Error(err)
	}
	if req.Body != nil && req.ContentType != "" {
		hreq.Header.Set("Content-Type", req.ContentType)
	}
	resp, err := t.client.Do(hreq)
	if err != nil {
		// net/http has already closed req.Body.
		return nil, sanitize.Error(err)
	}
	defer closeBody(resp.Body)

	body := io.Reader(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body = io.LimitReader(resp.Body, maxErrorBody)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, sanitize.Error(fmt.Errorf("teleiq: %s: reading response: %w", req.Method, err))
	}
	out := &Response{StatusCode: resp.StatusCode, Body: data}
	if resp.StatusCode == http.StatusTooManyRequests {
		out.RetryAfter = retryAfterHeader(resp.Header.Get("Retry-After"), time.Now())
	}
	return out, nil
}

// fileDownloader is implemented by transports that can stream files to a writer.
type fileDownloader interface {
	download(ctx context.Context, filePath string, dst io.Writer) error
}

func (t *httpTransport) download(ctx context.Context, filePath string, dst io.Writer) error {
	path, err := fileURLPath(filePath)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/file/bot"+t.token+"/"+path, nil)
	if err != nil {
		return sanitize.Error(err)
	}
	resp, err := t.client.Do(hreq)
	if err != nil {
		return sanitize.Error(fmt.Errorf("teleiq: download: %w", err))
	}
	defer closeBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		err := decodeResponse("download", &Response{StatusCode: resp.StatusCode, Body: data}, nil)
		if err == nil {
			err = &Error{ErrorCode: resp.StatusCode, Description: http.StatusText(resp.StatusCode), Method: "download"}
		}
		return err
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return sanitize.Error(fmt.Errorf("teleiq: download: %w", err))
	}
	return nil
}

// fileURLPath escapes the segments of a getFile path, rejecting unsafe ones.
func fileURLPath(filePath string) (string, error) {
	segments := strings.Split(filePath, "/")
	for i, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return "", sanitize.Error(fmt.Errorf("teleiq: download: invalid file path %q", filePath))
		}
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/"), nil
}

func validMethod(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

func closeBody(body io.Reader) {
	if c, ok := body.(io.Closer); ok {
		_ = c.Close()
	}
}

func newDefaultHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	// No client-wide Timeout, so large uploads and downloads are not cut off.
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100, // every request goes to the same Bot API host
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}
