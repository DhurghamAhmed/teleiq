package teleiq

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Option configures a Client created by NewClient.
type Option func(*options) error

type options struct {
	httpClient     *http.Client
	transport      Transport
	defaultTimeout time.Duration
	retry          RetryPolicy
	limiter        RateLimiter
	baseURL        string
	logger         *slog.Logger
	defaults       *Defaults
}

// WithDefaultTimeout sets the timeout for calls whose context has no deadline.
func WithDefaultTimeout(d time.Duration) Option {
	return func(o *options) error {
		if d < 0 {
			return errors.New("teleiq: WithDefaultTimeout: negative duration")
		}
		o.defaultTimeout = d
		return nil
	}
}

// WithRetryPolicy sets how failed requests are retried; the default is Backoff{}.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(o *options) error {
		if p == nil {
			return errors.New("teleiq: WithRetryPolicy: nil policy")
		}
		o.retry = p
		return nil
	}
}

// WithRateLimiter makes every request wait for l before it is sent.
func WithRateLimiter(l RateLimiter) Option {
	return func(o *options) error {
		if l == nil {
			return errors.New("teleiq: WithRateLimiter: nil limiter")
		}
		o.limiter = l
		return nil
	}
}

// WithBaseURL sends requests to a Bot API server other than api.telegram.org.
func WithBaseURL(rawURL string) Option {
	return func(o *options) error {
		if err := checkBaseURL(rawURL); err != nil {
			// The errors repeat the URL, which could hold a token pasted into it by mistake.
			return sanitize.Error(err)
		}
		o.baseURL = strings.TrimRight(rawURL, "/")
		return nil
	}
}

func checkBaseURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	switch {
	case err != nil:
		return fmt.Errorf("teleiq: WithBaseURL: %w", err)
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("teleiq: WithBaseURL: %q is not an http or https URL", rawURL)
	case u.Host == "":
		return fmt.Errorf("teleiq: WithBaseURL: %q has no host", rawURL)
	case u.User != nil:
		return errors.New("teleiq: WithBaseURL: the URL cannot hold credentials; set them with WithHTTPClient")
	case u.RawQuery != "" || u.Fragment != "" || u.ForceQuery:
		return fmt.Errorf("teleiq: WithBaseURL: %q cannot have a query or a fragment", rawURL)
	}
	return nil
}

// WithLogger logs requests, retries, flood waits and downloads to l.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) error {
		if l == nil {
			return errors.New("teleiq: WithLogger: nil logger")
		}
		o.logger = l
		return nil
	}
}

// WithHTTPClient sends requests through c, which is used as is.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) error {
		if c == nil {
			return errors.New("teleiq: WithHTTPClient: nil client")
		}
		o.httpClient = c
		return nil
	}
}

// WithTransport replaces how requests reach the Bot API.
func WithTransport(t Transport) Option {
	return func(o *options) error {
		if t == nil {
			return errors.New("teleiq: WithTransport: nil transport")
		}
		o.transport = t
		return nil
	}
}

// Defaults are values a Client sends for the parameters that a call leaves unset.
type Defaults struct {
	// ParseMode is the default parse_mode of the methods that have one.
	ParseMode string

	// LinkPreviewOptions are the default link_preview_options of text messages.
	LinkPreviewOptions *models.LinkPreviewOptions

	// DisableNotification sends silently; ProtectContent blocks forwarding and saving.
	DisableNotification bool
	ProtectContent      bool
}

// WithDefaults makes the Client send d for the parameters that a call leaves nil.
func WithDefaults(d Defaults) Option {
	return func(o *options) error {
		if d.LinkPreviewOptions != nil {
			lp := *d.LinkPreviewOptions // a copy, which the caller cannot change later
			d.LinkPreviewOptions = &lp
		}
		if d == (Defaults{}) {
			o.defaults = nil
			return nil
		}
		o.defaults = &d
		return nil
	}
}
