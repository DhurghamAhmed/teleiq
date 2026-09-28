// Package sanitize removes Telegram bot tokens from text and errors.
package sanitize

import (
	"errors"
	"net/url"
	"regexp"
)

// Placeholder replaces each removed token.
const Placeholder = "<redacted>"

// tokenPattern matches bot tokens, including back-to-back ones.
var tokenPattern = regexp.MustCompile(`[0-9]{5,}(?:(?::|%3[Aa])[A-Za-z0-9_-]{30,})+`)

// String replaces every bot token in s with Placeholder.
func String(s string) string {
	return tokenPattern.ReplaceAllLiteralString(s, Placeholder)
}

// Error removes bot tokens from err and every error it wraps.
func Error(err error) error {
	if err == nil || !tokenPattern.MatchString(err.Error()) {
		return err
	}
	if ue, ok := err.(*url.Error); ok {
		return &url.Error{Op: ue.Op, URL: String(ue.URL), Err: Error(ue.Err)}
	}
	return &redactedError{msg: String(err.Error()), err: err}
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }

func (e *redactedError) Unwrap() error { return Error(errors.Unwrap(e.err)) }
