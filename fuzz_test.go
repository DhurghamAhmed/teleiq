package teleiq

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

var fuzzUpdates = []string{
	`{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"/start",` +
		`"entities":[{"type":"bot_command","offset":0,"length":6}],"forward_origin":{"type":"user","date":1,` +
		`"sender_user":{"id":2,"is_bot":false,"first_name":"A"}},"reply_to_message":{"message_id":0,"date":1,` +
		`"chat":{"id":1,"type":"private"}},"pinned_message":{"message_id":3,"date":0,"chat":{"id":1,"type":"private"}}}}`,
	`{"update_id":2,"callback_query":{"id":"q","chat_instance":"c","from":{"id":2,"is_bot":false,"first_name":"A"},` +
		`"data":"x","message":{"message_id":3,"date":0,"chat":{"id":-100,"type":"supergroup"}}}}`,
	`{"update_id":3,"chat_member":{"chat":{"id":-100,"type":"group"},"from":{"id":2,"is_bot":false,"first_name":"A"},` +
		`"date":1,"old_chat_member":{"status":"left","user":{"id":3,"is_bot":false,"first_name":"B"}},` +
		`"new_chat_member":{"status":"a status from the future","user":{"id":3,"is_bot":false,"first_name":"B"}}}}`,
	`{"update_id":4,"message_reaction":{"chat":{"id":1,"type":"private"},"message_id":1,"date":1,` +
		`"old_reaction":[],"new_reaction":[{"type":"emoji","emoji":"👍"},{"type":"paid"},{"type":"new_kind","x":[1]}]}}`,
}

// FuzzResponse checks that any reply of the Bot API gives a result or an error, never a success
// for a reply that is not ok, and never an error that shows a token.
func FuzzResponse(f *testing.F) {
	f.Add(200, int64(0), []byte(`{"ok":true,"result":[`+fuzzUpdates[0]+`]}`))
	f.Add(429, int64(3*time.Second), []byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":5}}`))
	f.Add(502, int64(0), []byte(`<html>Bad Gateway</html>`))
	f.Add(200, int64(0), []byte(`{"ok":false,"description":"Unauthorized"}`))
	f.Add(200, int64(0), []byte(`{"ok":false,"result":[]}`))
	f.Add(200, int64(0), []byte(`{"ok":true,"result":[{"update_id":"1"}]}`))
	f.Add(400, int64(0), []byte(`{"ok":false,"error_code":400,"description":"bad token `+testToken+`"}`))
	f.Fuzz(func(t *testing.T, status int, retryAfter int64, body []byte) {
		c, err := NewClient(testToken, WithRetryPolicy(Backoff{MaxAttempts: 1}),
			WithTransport(TransportFunc(func(context.Context, *Request) (*Response, error) {
				return &Response{StatusCode: status, Body: body, RetryAfter: time.Duration(retryAfter)}, nil
			})))
		if err != nil {
			t.Fatal(err)
		}
		var updates []models.Update
		err = c.Call(context.Background(), "getUpdates", nil, &updates)
		if err == nil {
			var env struct{ OK bool }
			if json.Unmarshal(body, &env) != nil || !env.OK {
				t.Fatalf("Call() succeeded on a reply that is not ok: %q", body)
			}
			return
		}
		if msg := err.Error(); sanitize.String(msg) != msg {
			t.Errorf("error shows a token: %q", msg)
		}
		if apiErr, ok := errors.AsType[*Error](err); ok && apiErr.Method != "getUpdates" {
			t.Errorf("Error.Method = %q, want getUpdates", apiErr.Method)
		}
	})
}

// resultTypes returns the result type of every method of Client.
func resultTypes() []reflect.Type {
	var out []reflect.Type
	for m := range reflect.TypeFor[*Client]().Methods() {
		if m.Type.NumOut() == 2 && !slices.Contains(out, m.Type.Out(0)) {
			out = append(out, m.Type.Out(0))
		}
	}
	slices.SortFunc(out, func(a, b reflect.Type) int { return strings.Compare(a.String(), b.String()) })
	return out
}

// variants appends the concrete types held by the interfaces in v, in order.
func variants(v reflect.Value, out []reflect.Type) []reflect.Type {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			out = variants(v.Elem(), append(out, v.Elem().Type()))
		}
	case reflect.Pointer:
		if !v.IsNil() {
			out = variants(v.Elem(), out)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			out = variants(v.Field(i), out)
		}
	case reflect.Slice:
		for i := range v.Len() {
			out = variants(v.Index(i), out)
		}
	}
	return out
}

// FuzzResult decodes JSON into each result type and checks that what it decodes encodes and
// decodes again to the same value, so that no variant of a union is lost or changed on the way.
func FuzzResult(f *testing.F) {
	types := resultTypes()
	for i, typ := range types {
		sample := map[reflect.Kind]string{reflect.Bool: `true`, reflect.String: `"text"`, reflect.Int64: `3`, reflect.Slice: `[{}]`}[typ.Kind()]
		f.Add(uint16(i), []byte(cmp.Or(sample, `{}`)))
	}
	updates := slices.Index(types, reflect.TypeFor[[]models.Update]())
	for _, u := range fuzzUpdates {
		f.Add(uint16(updates), []byte("["+u+"]"))
	}
	f.Fuzz(func(t *testing.T, i uint16, data []byte) {
		typ := types[int(i)%len(types)]
		v := reflect.New(typ)
		if json.Unmarshal(data, v.Interface()) != nil {
			return
		}
		first, err := json.Marshal(v.Interface())
		if err != nil {
			t.Fatalf("%s: encoding what was decoded from %q: %v", typ, data, err)
		}
		again := reflect.New(typ)
		if err := json.Unmarshal(first, again.Interface()); err != nil {
			t.Fatalf("%s: decoding %q, which it encoded: %v", typ, first, err)
		}
		if want, got := variants(v, nil), variants(again, nil); !slices.Equal(want, got) {
			t.Fatalf("%s: the variants %v became %v on a round trip through %s", typ, want, got, first)
		}
		second, err := json.Marshal(again.Interface())
		if err != nil {
			t.Fatalf("%s: encoding again: %v", typ, err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("%s changed on a round trip:\n%s\n%s", typ, first, second)
		}
	})
}

// FuzzFileURLPath checks that a file path from getFile is either rejected or escaped into a path
// that stays under the file URL of the bot and names the same file.
func FuzzFileURLPath(f *testing.F) {
	for _, p := range []string{"documents/file_1.txt", "photos/my photo#1.jpg", "a/../b", "/abs", "a//b", "%2e%2e/x", "a?b=c", `a\b`} {
		f.Add(p)
	}
	base := "https://api.telegram.org/file/bot" + testToken + "/"
	f.Fuzz(func(t *testing.T, filePath string) {
		escaped, err := fileURLPath(filePath)
		segments := strings.Split(filePath, "/")
		invalid := slices.ContainsFunc(segments, func(s string) bool { return s == "" || s == "." || s == ".." })
		if err != nil {
			if !invalid {
				t.Fatalf("fileURLPath(%q) rejected a valid path: %v", filePath, err)
			}
			return
		}
		if invalid {
			t.Fatalf("fileURLPath(%q) = %q, want an error", filePath, escaped)
		}
		u, err := url.Parse(base + escaped)
		if err != nil {
			t.Fatalf("the URL of %q does not parse: %v", filePath, err)
		}
		rest, ok := strings.CutPrefix(u.EscapedPath(), "/file/bot"+testToken+"/")
		if !ok || u.RawQuery != "" || u.Fragment != "" || u.Host != "api.telegram.org" {
			t.Fatalf("fileURLPath(%q) leaves the file URL: %s", filePath, u)
		}
		got := strings.Split(rest, "/")
		for i, seg := range got {
			if seg, err = url.PathUnescape(seg); err != nil || seg != segments[i] {
				t.Fatalf("fileURLPath(%q) = %q, which names another file", filePath, escaped)
			}
		}
	})
}

// FuzzRetryAfterHeader checks that no Retry-After header gives a negative wait.
func FuzzRetryAfterHeader(f *testing.F) {
	for _, v := range []string{"5", "-5", "0", "99999999999999999999", "Wed, 21 Oct 2015 07:28:00 GMT", "soon"} {
		f.Add(v)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, v string) {
		if d := retryAfterHeader(v, now); d < 0 {
			t.Errorf("retryAfterHeader(%q) = %v, want no negative wait", v, d)
		}
	})
}
