package sanitize

import (
	"strings"
	"testing"
)

// FuzzString checks that a token is removed whatever surrounds it, and that no text that looks
// like a token is left.
func FuzzString(f *testing.F) {
	f.Add("https://api.telegram.org/bot", "/getMe")
	f.Add("Post \"https://api.telegram.org/bot", "/sendMessage\": EOF")
	f.Add("98765", "")
	f.Add(otherToken, "")
	f.Add("111111:"+strings.Repeat("x", 30), "")
	f.Add("", otherSecret)
	f.Fuzz(func(t *testing.T, before, after string) {
		if strings.Contains(before, secret) || strings.Contains(after, secret) {
			t.Skip()
		}
		for _, tok := range []string{token, strings.Replace(token, ":", "%3A", 1)} {
			out := String(before + tok + after)
			if strings.Contains(out, secret) {
				t.Errorf("String(%q) = %q, which still has the secret", before+tok+after, out)
			}
			if String(out) != out {
				t.Errorf("String(%q) = %q, which still has a token", before+tok+after, out)
			}
		}
	})
}
