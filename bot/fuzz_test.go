package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/DhurghamAhmed/teleiq/models"
)

// FuzzCallback checks that the fields that Pack writes come back from Unpack as they were, whatever
// colons or backslashes they hold, and that a press of the button matches its Callback.
func FuzzCallback(f *testing.F) {
	for _, s := range [][3]string{{"buy", "42", "5"}, {"a:b", `c\`, ":"}, {"", `\:`, ""}, {"x", "", `\`}} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, prefix, a, b string) {
		cb := NewCallback(prefix)
		data, err := cb.Pack(a, b)
		if err != nil {
			if len(data) > 0 && len(data) <= 64 {
				t.Fatalf("Pack(%q, %q) = %q, %v for data within 64 bytes", a, b, data, err)
			}
			return
		}
		fields := cb.Unpack(data)
		if got1, got2 := fields.String(0), fields.String(1); got1 != a || got2 != b || fields.Len() != 2 || fields.Err() != nil {
			t.Fatalf("Unpack(%q) = %q, %q, %d fields, %v; want %q, %q", data, got1, got2, fields.Len(), fields.Err(), a, b)
		}
		if !cb.Match(&Context{update: &models.Update{CallbackQuery: &models.CallbackQuery{Data: &data}}}) {
			t.Fatalf("Match(%q) = false for a button of the Callback", data)
		}
	})
}

// FuzzParseCommand checks that a command is parsed into parts that come from the text, and that
// every kind of white space ends a command the same way.
func FuzzParseCommand(f *testing.F) {
	for _, s := range []string{"/start", "/start@test_bot hello  world ", "/start\r\nhi", "/echo text", "/help@bot\tx",
		"hello", "/", "/@bot", "/" + strings.Repeat("a", 33), "/start@", " /start", "/start@a@b c"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		name, mention, args, ok := parseCommand(text)
		if !ok {
			if name != "" || mention != "" || args != "" {
				t.Fatalf("parseCommand(%q) = %q, %q, %q for no command", text, name, mention, args)
			}
		} else if !validCommand(name) || !strings.HasPrefix(text, "/"+name) || strings.ContainsFunc(mention, unicode.IsSpace) ||
			args != strings.TrimSpace(args) || !strings.HasSuffix(strings.TrimRightFunc(text, unicode.IsSpace), args) {
			t.Fatalf("parseCommand(%q) = %q, %q, %q", text, name, mention, args)
		}
		if i := strings.IndexFunc(text, unicode.IsSpace); i >= 0 {
			_, size := utf8.DecodeRuneInString(text[i:])
			spaced := text[:i] + " " + text[i+size:]
			n, m, a, o := parseCommand(spaced)
			if n != name || m != mention || a != args || o != ok {
				t.Fatalf("parseCommand(%q) = %q, %q, %q, %v but parseCommand(%q) = %q, %q, %q, %v",
					text, name, mention, args, ok, spaced, n, m, a, o)
			}
		}
	})
}

// jsonDepth returns how deeply the valid JSON data nests objects and arrays.
func jsonDepth(data []byte) int {
	dec := jsontext.NewDecoder(bytes.NewReader(data), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	deepest := 0
	for {
		if _, err := dec.ReadToken(); err != nil {
			return deepest
		}
		deepest = max(deepest, dec.StackDepth())
	}
}

// FuzzWebhook checks that the webhook handles every body that is an update and rejects the rest.
func FuzzWebhook(f *testing.F) {
	nested := func(n int) []byte {
		return []byte(`{"update_id":1,"x":` + strings.Repeat("[", n-1) + strings.Repeat("]", n-1) + `}`)
	}
	for _, s := range [][]byte{
		[]byte(update(1, 1)),
		[]byte(`{"update_id":2,"callback_query":{"id":"q","chat_instance":"c","from":{"id":2,"is_bot":false,"first_name":"A"},` +
			`"message":{"message_id":3,"date":0,"chat":{"id":-100,"type":"supergroup"}}}}`),
		[]byte(`{"update_id":3,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"/start@test_bot [{\"\\"}]"}}`),
		[]byte(`{"update_id":4,"inline_query":{"id":"i","from":{"id":7,"is_bot":false,"first_name":"U"},"query":"","offset":""}}`),
		[]byte(`{"update_id":5,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"\\\"` + strings.Repeat("[", maxDepth) + `"}}`),
		[]byte(`{}`), []byte(`null`), []byte(`[]`), []byte(`{"update_id":1} {}`), []byte(`{"update_id":"1"}`),
		nested(maxDepth), nested(maxDepth + 1),
	} {
		f.Add(s)
	}
	b, _ := newTestBot(f, newFakeTelegram())
	var handled atomic.Int64
	b.Handle(FilterFunc(func(c *Context) bool {
		_, _, _, _, _ = c.Update(), c.Message(), c.Chat(), c.Sender(), c.Command()+c.Args()
		return true
	}), func(context.Context, *Context) error {
		handled.Add(1)
		return nil
	})
	stop := startWebhook(f, b)
	f.Cleanup(func() { _ = stop() })
	handler := b.WebhookHandler()

	f.Fuzz(func(t *testing.T, body []byte) {
		var u models.Update
		var fields map[string]json.RawMessage
		valid := json.Unmarshal(body, &u) == nil && json.Unmarshal(body, &fields) == nil && fields != nil &&
			jsonDepth(body) <= maxDepth
		before := handled.Load()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
		switch {
		case valid && (rec.Code != http.StatusOK || handled.Load() != before+1):
			t.Fatalf("an update got %d and was handled %d times: %q", rec.Code, handled.Load()-before, body)
		case !valid && rec.Code != http.StatusBadRequest:
			t.Fatalf("a body that is not an update got %d: %q", rec.Code, body)
		}
	})
}
