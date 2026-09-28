package teleiq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

type logRecord struct {
	level, msg string
	attrs      map[string]any
}

// captureLogs returns a logger at level and a function that reads what it logged.
func captureLogs(t *testing.T, level slog.Level) (*slog.Logger, func() (string, []logRecord)) {
	t.Helper()
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	return l, func() (string, []logRecord) {
		var records []logRecord
		for line := range strings.Lines(buf.String()) {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("log line %q: %v", line, err)
			}
			r := logRecord{level: m["level"].(string), msg: m["msg"].(string), attrs: m}
			delete(m, "time")
			delete(m, "level")
			delete(m, "msg")
			records = append(records, r)
		}
		return buf.String(), records
	}
}

// summary lists each record as "LEVEL msg" with the chosen attributes.
func summary(records []logRecord, keys ...string) []string {
	var out []string
	for _, r := range records {
		s := r.level + " " + r.msg
		for _, k := range keys {
			if v, ok := r.attrs[k]; ok {
				s += " " + k + "=" + fmt.Sprint(v)
			}
		}
		out = append(out, s)
	}
	return out
}

func TestLogging(t *testing.T) {
	message := `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":12345,"type":"private"},"text":"secret reply"}}`
	tests := []struct {
		name    string
		level   slog.Level
		steps   []step
		limiter RateLimiter
		before  func(c *Client)
		call    func(ctx context.Context, c *Client) error
		want    []string
	}{
		{
			name: "request", level: slog.LevelDebug, steps: []step{{status: 200, reply: okTrue}},
			call: func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			want: []string{"DEBUG teleiq: request method=logOut attempt=1"},
		},
		{
			name: "retry", level: slog.LevelDebug, steps: []step{{status: 502, reply: "<html>Bad Gateway</html>"}, {status: 200, reply: okTrue}},
			call: func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			want: []string{
				"DEBUG teleiq: request method=logOut attempt=1 error_code=502 error=teleiq: logOut: Bad Gateway (502)",
				"WARN teleiq: retrying request method=logOut attempt=1 error_code=502 error=teleiq: logOut: Bad Gateway (502)",
				"DEBUG teleiq: request method=logOut attempt=2",
			},
		},
		{
			name: "only warnings at the Info level", level: slog.LevelInfo, steps: []step{{status: 502, reply: "x"}, {status: 200, reply: okTrue}},
			call: func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			want: []string{"WARN teleiq: retrying request method=logOut attempt=1 error_code=502 error=teleiq: logOut: Bad Gateway (502)"},
		},
		{
			name: "token removed from errors", level: slog.LevelDebug,
			steps: []step{{err: errors.New("dial tcp: lookup /bot" + testToken + "/logOut: no such host")}},
			call:  func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			want: []string{
				"DEBUG teleiq: request method=logOut attempt=1 error=teleiq: logOut: dial tcp: lookup /bot<redacted>/logOut: no such host",
				"WARN teleiq: retrying request method=logOut attempt=1 error=teleiq: logOut: dial tcp: lookup /bot<redacted>/logOut: no such host",
				"DEBUG teleiq: request method=logOut attempt=2 error=teleiq: logOut: dial tcp: lookup /bot<redacted>/logOut: no such host",
			},
		},
		{
			name: "token removed from other errors", level: slog.LevelDebug, steps: []step{{status: 200, reply: okTrue}},
			limiter: &recordingLimiter{err: errors.New("quota of " + testToken + " used up")},
			call:    func(ctx context.Context, c *Client) error { return c.LogOut(ctx) },
			want:    []string{"DEBUG teleiq: request method=logOut attempt=1 error=teleiq: logOut: rate limiter: quota of <redacted> used up"},
		},
		{
			name: "flood control wait", level: slog.LevelDebug, steps: []step{{status: 200, reply: message}},
			before: func(c *Client) { c.cooldown.block(models.ID(12345), 20*time.Millisecond) },
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SendMessage(ctx, SendMessageParams{ChatID: models.ID(12345), Text: "secret message"})
				return err
			},
			want: []string{"WARN teleiq: waited for flood control method=sendMessage", "DEBUG teleiq: request method=sendMessage attempt=1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := captureLogs(t, tt.level)
			tr := &scriptedTransport{steps: tt.steps}
			opts := []Option{WithLogger(logger), WithRetryPolicy(Backoff{MaxAttempts: 2, BaseDelay: time.Millisecond})}
			if tt.limiter != nil {
				opts = append(opts, WithRateLimiter(tt.limiter))
			}
			c := newTestClient(t, tr, opts...)
			if tt.before != nil {
				tt.before(c)
			}

			err := tt.call(context.Background(), c)
			if err != nil && strings.Contains(err.Error(), testToken) {
				t.Errorf("error %q contains the token", err)
			}

			text, records := logs()
			got := summary(records, "method", "attempt", "error_code", "error")
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("logs =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			for _, secret := range []string{testToken, "secret message", "secret reply", "12345"} {
				if strings.Contains(text, secret) {
					t.Errorf("logs contain %q:\n%s", secret, text)
				}
			}
			for _, r := range records {
				if _, ok := r.attrs["duration"]; !ok && r.msg != "teleiq: retrying request" {
					t.Errorf("record %q has no duration", r.msg)
				}
			}
		})
	}
}

func TestLoggingDownload(t *testing.T) {
	logger, logs := captureLogs(t, slog.LevelDebug)
	c, _ := newFileServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/documents/a.txt") {
			_, _ = io.WriteString(w, "content")
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":404,"description":"Not Found"}`)
	})
	c.log = logger

	_ = c.Download(context.Background(), "documents/a.txt", io.Discard)
	_ = c.Download(context.Background(), "documents/none.txt", io.Discard)

	text, records := logs()
	want := []string{
		"DEBUG teleiq: download file_path=documents/a.txt",
		"DEBUG teleiq: download file_path=documents/none.txt error_code=404 error=teleiq: download: Not Found (404)",
	}
	if got := summary(records, "file_path", "error_code", "error"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("logs =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Contains(text, testToken) {
		t.Errorf("logs contain the token:\n%s", text)
	}
}

func TestLoggingIsOffByDefault(t *testing.T) {
	c, err := NewClient(testToken)
	if err != nil {
		t.Fatal(err)
	}
	if c.log.Enabled(context.Background(), slog.LevelError) {
		t.Error("a Client without WithLogger logs")
	}
}
