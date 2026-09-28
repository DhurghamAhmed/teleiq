package teleiq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
	"github.com/DhurghamAhmed/teleiq/internal/upload"
	"github.com/DhurghamAhmed/teleiq/models"
)

const (
	defaultBaseURL        = "https://api.telegram.org"
	defaultRequestTimeout = 30 * time.Second
)

// Client calls the Telegram Bot API; it is safe for concurrent use.
type Client struct {
	transport      Transport
	defaultTimeout time.Duration
	retry          RetryPolicy
	limiter        RateLimiter
	cooldown       *cooldowns
	log            *slog.Logger
	defaults       *Defaults // nil without WithDefaults
}

// NewClient returns a Client for the bot with the given token.
func NewClient(token string, opts ...Option) (*Client, error) {
	if !validToken(token) {
		return nil, ErrInvalidToken
	}
	o := options{defaultTimeout: defaultRequestTimeout, retry: Backoff{}, logger: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	if o.transport != nil && (o.httpClient != nil || o.baseURL != "") {
		return nil, errors.New("teleiq: WithTransport cannot be combined with WithHTTPClient or WithBaseURL")
	}
	if o.baseURL == "" {
		o.baseURL = defaultBaseURL
	}
	t := o.transport
	if t == nil {
		hc := o.httpClient
		if hc == nil {
			hc = newDefaultHTTPClient()
		}
		t = newHTTPTransport(hc, o.baseURL, token)
	}
	return &Client{
		transport:      t,
		defaultTimeout: o.defaultTimeout,
		retry:          o.retry,
		limiter:        o.limiter,
		cooldown:       &cooldowns{},
		log:            o.logger,
		defaults:       o.defaults,
	}, nil
}

// Call invokes a Bot API method by name and decodes its result into result.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if c == nil || c.transport == nil {
		return errors.New("teleiq: Client must be created with NewClient")
	}
	if ctx == nil {
		return errors.New("teleiq: nil Context")
	}
	if !validMethod(method) {
		return sanitize.Error(fmt.Errorf("teleiq: invalid method name %q", method))
	}
	var body *upload.Body
	if params != nil {
		var err error
		if body, err = encodeBody(params); err != nil {
			return sanitize.Error(fmt.Errorf("teleiq: %s: encoding parameters: %w", method, err))
		}
		// Uploaded files are closed when Call returns, unless it returns because ctx ended.
		defer body.Stop(ctx)
	}
	// A caller's deadline wins; uploads get no default so large files are not cut off.
	if _, ok := ctx.Deadline(); !ok && c.defaultTimeout > 0 && (body == nil || !body.HasUploads()) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.defaultTimeout)
		defer cancel()
	}
	chat := targetOf(params)
	for attempt := 1; ; attempt++ {
		start := time.Now()
		network, err := c.attempt(ctx, method, chat, body, result)
		// Errors from caller code, such as a rate limiter, may hold a token.
		err = sanitize.Error(err)
		c.logAttempt(ctx, method, attempt, time.Since(start), err)
		if err == nil {
			return nil
		}
		delay, retry := c.retryDelay(ctx, attempt, err, network)
		if !retry || (body != nil && !body.Replayable()) {
			return err
		}
		c.logRetry(ctx, method, attempt, delay, err)
		if !sleep(ctx, delay) {
			return err
		}
	}
}

// attempt sends the request once; network reports a transport failure.
func (c *Client) attempt(ctx context.Context, method string, chat models.ChatID, body *upload.Body, result any) (network bool, err error) {
	waited, err := c.cooldown.wait(ctx, method, chat)
	if waited > 0 {
		c.logFloodWait(ctx, method, waited)
	}
	if err != nil {
		return false, err
	}
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx, method, chat); err != nil {
			return false, fmt.Errorf("teleiq: %s: rate limiter: %w", method, err)
		}
	}
	req := &Request{Method: method}
	if body != nil {
		if req.Body, err = body.Open(); err != nil {
			return false, fmt.Errorf("teleiq: %s: opening files: %w", method, err)
		}
		// Closing the body stops an upload's writer even if the transport never read it.
		defer closeBody(req.Body)
		req.ContentType = body.ContentType()
	}
	resp, err := c.transport.Do(ctx, req)
	if err != nil {
		return true, fmt.Errorf("teleiq: %s: %w", method, err)
	}
	if resp == nil {
		return false, fmt.Errorf("teleiq: %s: transport returned no response", method)
	}
	err = decodeResponse(method, resp, result)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.ErrorCode == http.StatusTooManyRequests && apiErr.Parameters != nil && apiErr.Parameters.RetryAfter != nil {
		c.cooldown.block(chat, seconds(int64(*apiErr.Parameters.RetryAfter)))
	}
	return false, err
}

type apiResponse struct {
	OK          bool                       `json:"ok"`
	Result      json.RawMessage            `json:"result"`
	ErrorCode   int                        `json:"error_code"`
	Description string                     `json:"description"`
	Parameters  *models.ResponseParameters `json:"parameters"`
}

func decodeResponse(method string, resp *Response, result any) error {
	var env apiResponse
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		// Proxies and gateways can answer with error pages that are not JSON.
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			apiErr := &Error{ErrorCode: resp.StatusCode, Description: http.StatusText(resp.StatusCode), Method: method}
			if resp.StatusCode == http.StatusTooManyRequests && resp.RetryAfter > 0 {
				apiErr.Parameters = withRetryAfter(nil, resp.RetryAfter)
			}
			return apiErr
		}
		return fmt.Errorf("teleiq: %s: decoding response: %w", method, err)
	}
	if !env.OK {
		code := env.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		params := env.Parameters
		if code == http.StatusTooManyRequests && resp.RetryAfter > 0 && (params == nil || params.RetryAfter == nil) {
			params = withRetryAfter(params, resp.RetryAfter)
		}
		return &Error{ErrorCode: code, Description: sanitize.String(env.Description), Parameters: params, Method: method}
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(env.Result, result); err != nil {
		return fmt.Errorf("teleiq: %s: decoding result: %w", method, err)
	}
	return nil
}

func validToken(token string) bool {
	id, secret, ok := strings.Cut(token, ":")
	if !ok || id == "" || secret == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range secret {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// Ptr returns a pointer to a copy of v, for setting optional fields.
func Ptr[T any](v T) *T {
	return &v
}

// decodeOrTrue decodes a result that is either an object or true.
func decodeOrTrue[T any](method string, raw json.RawMessage) (*T, error) {
	if string(bytes.TrimSpace(raw)) == "true" {
		return nil, nil
	}
	v := new(T)
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, fmt.Errorf("teleiq: %s: decoding result: %w", method, err)
	}
	return v, nil
}

// decodeUnion decodes a result that can hold several types.
func decodeUnion[T any](method string, raw json.RawMessage, decode func([]byte) (T, error)) (T, error) {
	v, err := decode(raw)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("teleiq: %s: decoding result: %w", method, err)
	}
	return v, nil
}

// decodeUnions decodes an array result whose values can hold several types.
func decodeUnions[T any](method string, raw json.RawMessage, decode func([]byte) (T, error)) ([]T, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(raw, &raws); err != nil {
		return nil, fmt.Errorf("teleiq: %s: decoding result: %w", method, err)
	}
	if raws == nil {
		return nil, nil
	}
	vs := make([]T, len(raws))
	for i, r := range raws {
		v, err := decode(r)
		if err != nil {
			return nil, fmt.Errorf("teleiq: %s: decoding result: %w", method, err)
		}
		vs[i] = v
	}
	return vs, nil
}
