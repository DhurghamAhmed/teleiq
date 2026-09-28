package bot

import (
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

func TestCallbackPack(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		fields  []any
		want    string
		wantErr bool
	}{
		{"prefix only", "menu", nil, "menu", false},
		{"integers", "buy", []any{int64(42), 5, uint8(7), int32(-1)}, "buy:42:5:7:-1", false},
		{"string and bool", "set", []any{"lang", true, false}, "set:lang:1:0", false},
		{"colon in a field", "note", []any{"a:b"}, `note:a\:b`, false},
		{"backslash in a field", "path", []any{`c:\x`}, `path:c\:\\x`, false},
		{"colon in the prefix", "a:b", []any{1}, `a\:b:1`, false},
		{"empty field", "x", []any{""}, "x:", false},
		{"64 bytes", "p", []any{strings.Repeat("a", 62)}, "p:" + strings.Repeat("a", 62), false},
		{"65 bytes", "p", []any{strings.Repeat("a", 63)}, "p:" + strings.Repeat("a", 63), true},
		{"escapes count", "p", []any{strings.Repeat(":", 31)}, "p:" + strings.Repeat(`\:`, 31), false},
		{"empty data", "", nil, "", true},
		{"unsupported type", "p", []any{1.5}, "", true},
		{"nil field", "p", []any{nil}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCallback(tt.prefix).Pack(tt.fields...)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("Pack() = %q, %v; want %q, error: %t", got, err, tt.want, tt.wantErr)
			}
			if b := NewCallback(tt.prefix).Button("B", tt.fields...); b.CallbackData == nil || *b.CallbackData != tt.want {
				t.Errorf("Button() data = %v, want %q", b.CallbackData, tt.want)
			}
		})
	}
}

func TestCallbackUnpack(t *testing.T) {
	buy := NewCallback("buy")
	f := buy.Unpack("buy:42:5:a\\:b:1")
	if id, qty, note, gift := f.Int64(0), f.Int(1), f.String(2), f.Bool(3); id != 42 || qty != 5 || note != "a:b" || !gift || f.Err() != nil || f.Len() != 4 {
		t.Errorf("fields = %d, %d, %q, %t, err %v, len %d", id, qty, note, gift, f.Err(), f.Len())
	}

	tests := []struct {
		name string
		data string
		read func(f *CallbackFields)
		want string // in the error
	}{
		{"another prefix", "sell:42", func(f *CallbackFields) { f.Int64(0) }, "prefix"},
		{"a longer prefix", "buyer:42", func(f *CallbackFields) { f.Int64(0) }, "prefix"},
		{"an escaped colon after the prefix", `buy\:42`, func(f *CallbackFields) { f.Int64(0) }, "prefix"},
		{"a lone backslash", `buy:42\`, func(f *CallbackFields) { f.Int64(0) }, "backslash"},
		{"a missing field", "buy:42", func(f *CallbackFields) { f.Int64(0); f.Int(1) }, "not a field 1"},
		{"a negative index", "buy:42", func(f *CallbackFields) { f.String(-1) }, "not a field -1"},
		{"not a number", "buy:x", func(f *CallbackFields) { f.Int64(0) }, "field 0"},
		{"not a bool", "buy:2", func(f *CallbackFields) { f.Bool(0) }, "field 0"},
		{"the first error", "buy:x:y", func(f *CallbackFields) { f.Int(0); f.Int(1); f.String(9) }, "field 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := buy.Unpack(tt.data)
			tt.read(f)
			if f.Err() == nil || !strings.Contains(f.Err().Error(), tt.want) {
				t.Errorf("Err() = %v, want an error about %q", f.Err(), tt.want)
			}
		})
	}
	if f := buy.Unpack("buy:x"); f.Int(0) != 0 || f.String(0) != "x" {
		t.Errorf("a field that is not a number reads as 0 and still as its text")
	}
}

func TestCallbackMatch(t *testing.T) {
	press := func(data *string) *Context {
		return &Context{update: &models.Update{CallbackQuery: &models.CallbackQuery{Data: data}}}
	}
	s := func(v string) *string { return &v }
	tests := []struct {
		name   string
		prefix string
		c      *Context
		want   bool
	}{
		{"fields", "buy", press(s("buy:42:5")), true},
		{"prefix only", "buy", press(s("buy")), true},
		{"another prefix", "buy", press(s("sell:42")), false},
		{"a longer word", "buy", press(s("buyer:42")), false},
		{"an escaped colon", "buy", press(s(`buy\:42`)), false},
		{"an escaped prefix", "a:b", press(s(`a\:b:1`)), true},
		{"the unescaped prefix", "a:b", press(s("a:b:1")), false},
		{"no data", "buy", press(nil), false},
		{"a message", "buy", &Context{update: &models.Update{Message: &models.Message{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCallback(tt.prefix).Match(tt.c); got != tt.want {
				t.Errorf("Match() = %t, want %t", got, tt.want)
			}
		})
	}
}
