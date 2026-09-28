package sanitize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
)

// Built at run time so the source holds no token-like literal.
var (
	secret      = strings.Repeat("Ab1_-", 7)
	token       = "123456789:" + secret
	otherSecret = strings.Repeat("Zy9", 12)
	otherToken  = "987654321:" + otherSecret
)

func TestString(t *testing.T) {
	digest := strings.Repeat("0123456789abcdef", 4)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no token", "sendMessage failed: chat not found", "sendMessage failed: chat not found"},
		{"url", "https://api.telegram.org/bot" + token + "/getMe", "https://api.telegram.org/bot<redacted>/getMe"},
		{"file url", "https://api.telegram.org/file/bot" + token + "/photos/1.jpg", "https://api.telegram.org/file/bot<redacted>/photos/1.jpg"},
		{"bare token", "token=" + token, "token=<redacted>"},
		{"percent-encoded colon", "/bot123456789%3A" + secret + "/getMe", "/bot<redacted>/getMe"},
		{"two tokens", token + " and " + otherToken, "<redacted> and <redacted>"},
		{"hash digest kept", "sha256:" + digest, "sha256:" + digest},
		{"clock time kept", "at 12:34:56", "at 12:34:56"},
		{"short id kept", "chat 1234:abc", "chat 1234:abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.in); got != tt.want {
				t.Errorf("String(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestError(t *testing.T) {
	rawURL := "https://api.telegram.org/bot" + token + "/getMe"
	sentinel := errors.New("plain failure")
	urlErr := &url.Error{Op: "Post", URL: rawURL, Err: context.DeadlineExceeded}

	tests := []struct {
		name    string
		err     error
		same    bool
		is      error
		wantURL string
	}{
		{name: "nil", err: nil, same: true},
		{name: "clean error kept", err: sentinel, same: true, is: sentinel},
		{name: "clean wrapped error kept", err: fmt.Errorf("getMe: %w", context.Canceled), same: true, is: context.Canceled},
		{name: "url error", err: urlErr, is: context.DeadlineExceeded, wantURL: "https://api.telegram.org/bot<redacted>/getMe"},
		{name: "wrapped url error", err: fmt.Errorf("getMe: %w", urlErr), is: context.DeadlineExceeded, wantURL: "https://api.telegram.org/bot<redacted>/getMe"},
		{name: "token in message", err: fmt.Errorf("reading %s: %w", rawURL, io.ErrUnexpectedEOF), is: io.ErrUnexpectedEOF},
		{name: "token in inner message", err: fmt.Errorf("getMe: %w", fmt.Errorf("bad url %s", rawURL))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Error(tt.err)
			if tt.same && got != tt.err {
				t.Fatalf("Error returned %v, want the original error unchanged", got)
			}
			if tt.err == nil {
				return
			}
			if tt.is != nil && !errors.Is(got, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", got, tt.is)
			}
			for e := got; e != nil; e = errors.Unwrap(e) {
				if strings.Contains(e.Error(), secret) {
					t.Fatalf("error in chain leaks the token: %q", e.Error())
				}
			}
			if tt.wantURL != "" {
				var ue *url.Error
				if !errors.As(got, &ue) {
					t.Fatalf("errors.As(%v, *url.Error) = false, want true", got)
				}
				if ue.URL != tt.wantURL {
					t.Errorf("url.Error.URL = %q, want %q", ue.URL, tt.wantURL)
				}
				if !ue.Timeout() {
					t.Errorf("url.Error.Timeout() = false, want true for a deadline")
				}
			}
		})
	}
}
