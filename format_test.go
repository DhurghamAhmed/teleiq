package teleiq

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

// markdownReserved lists, one by one, the characters that the Bot API documentation of
// MarkdownV2, the Markdown of ParseModeMarkdown, requires to be escaped outside of entities, and
// the backslash.
var markdownReserved = []byte{
	'_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!', '\\',
}

func TestEscapeMarkdownEveryReservedCharacter(t *testing.T) {
	if len(markdownReserved) != 19 {
		t.Fatalf("the test lists %d characters, want 19", len(markdownReserved))
	}
	for _, c := range markdownReserved {
		t.Run(string(c), func(t *testing.T) {
			in := "a" + string(c) + "b"
			want := `a\` + string(c) + "b"
			if got := EscapeMarkdown(in); got != want {
				t.Errorf("EscapeMarkdown(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestEscapeMarkdownOtherBytesUnchanged(t *testing.T) {
	for i := range 256 {
		c := byte(i)
		if strings.IndexByte(string(markdownReserved), c) >= 0 {
			continue
		}
		if got := EscapeMarkdown(string(c)); got != string(c) {
			t.Errorf("EscapeMarkdown(%q) = %q, want it unchanged", c, got)
		}
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain text", in: "Hello world 42", want: "Hello world 42"},
		{name: "sentence", in: "Total: 1.5 (approx.)!", want: `Total: 1\.5 \(approx\.\)\!`},
		{name: "markup is shown as written", in: "*bold* _it_ [a](b)", want: `\*bold\* \_it\_ \[a\]\(b\)`},
		{name: "backslash already escaping", in: `\.`, want: `\\\.`},
		{name: "repeated", in: "--", want: `\-\-`},
		{name: "unicode", in: "مرحبا-😀.", want: `مرحبا\-😀\.`},
		{name: "invalid utf-8 kept", in: "\xff.\xfe", want: "\xff\\.\xfe"},
		{name: "every reserved character", in: string(markdownReserved), want: `\_\*\[\]\(\)\~` + "\\`" + `\>\#\+\-\=\|\{\}\.\!\\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeMarkdown(tt.in); got != tt.want {
				t.Errorf("EscapeMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscapeMarkdownPlainTextDoesNotAllocate(t *testing.T) {
	s := strings.Repeat("plain text ", 10)
	if n := testing.AllocsPerRun(100, func() { _ = EscapeMarkdown(s) }); n != 0 {
		t.Errorf("EscapeMarkdown of plain text allocates %v times, want 0", n)
	}
}

// FuzzEscapeMarkdown checks EscapeMarkdown against a reference: removing each escaping
// backslash gives back the input, and every reserved character of the result is escaped.
func FuzzEscapeMarkdown(f *testing.F) {
	for _, s := range []string{"", "plain", "a_b*c", `\`, `\\`, "1.5 (x)!", "a: b", string(markdownReserved), "\xff`"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := EscapeMarkdown(s)
		var unescaped strings.Builder
		for i := 0; i < len(got); i++ {
			c := got[i]
			if c == '\\' {
				if i+1 == len(got) || strings.IndexByte(string(markdownReserved), got[i+1]) < 0 {
					t.Fatalf("EscapeMarkdown(%q) = %q: backslash at %d does not escape a reserved character", s, got, i)
				}
				i++
				c = got[i]
			} else if strings.IndexByte(string(markdownReserved), c) >= 0 {
				t.Fatalf("EscapeMarkdown(%q) = %q: %q at %d is not escaped", s, got, c, i)
			}
			unescaped.WriteByte(c)
		}
		if unescaped.String() != s {
			t.Fatalf("unescaping EscapeMarkdown(%q) = %q gives %q", s, got, unescaped.String())
		}
	})
}

func TestPtr(t *testing.T) {
	type optional struct {
		B *bool   `json:"b,omitempty"`
		N *int64  `json:"n,omitempty"`
		S *string `json:"s,omitempty"`
	}
	tests := []struct {
		name string
		in   optional
		want string
	}{
		{name: "absent fields omitted", in: optional{}, want: `{}`},
		{name: "zero values sent", in: optional{B: Ptr(false), N: Ptr(int64(0)), S: Ptr("")}, want: `{"b":false,"n":0,"s":""}`},
		{name: "values sent", in: optional{B: Ptr(true), N: Ptr(int64(-5)), S: Ptr("x")}, want: `{"b":true,"n":-5,"s":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil || string(got) != tt.want {
				t.Fatalf("Marshal() = %s, %v; want %s", got, err, tt.want)
			}
		})
	}

	t.Run("points to a copy", func(t *testing.T) {
		v := 1
		p := Ptr(v)
		v = 2
		a, b := Ptr(v), Ptr(v)
		if *p != 1 || p == &v || a == b {
			t.Errorf("Ptr does not return a pointer to a fresh copy")
		}
	})
}

// bodies returns a client with opts whose requests are recorded in the returned map, by method.
func bodies(t *testing.T, opts ...Option) (*Client, map[string]string) {
	t.Helper()
	sent := map[string]string{}
	c := newTestClient(t, TransportFunc(func(_ context.Context, req *Request) (*Response, error) {
		b, _ := io.ReadAll(req.Body)
		sent[req.Method] = string(b)
		reply := `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":7,"type":"private"}}}`
		if req.Method == "copyMessage" {
			reply = `{"ok":true,"result":{"message_id":1}}`
		}
		return &Response{StatusCode: 200, Body: []byte(reply)}, nil
	}), opts...)
	return c, sent
}

func TestDefaults(t *testing.T) {
	ctx := context.Background()
	chat := models.ID(7)
	all := WithDefaults(Defaults{ParseMode: ParseModeHTML, LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: Ptr(true)},
		DisableNotification: true, ProtectContent: true})
	tests := []struct {
		name, method string
		opts         []Option
		call         func(c *Client) error
		want         string
	}{
		{"no defaults", "sendMessage", nil, func(c *Client) error {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi"})
			return err
		}, `{"chat_id":7,"text":"hi"}`},
		{"every default", "sendMessage", []Option{all}, func(c *Client) error {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi"})
			return err
		}, `{"chat_id":7,"text":"hi","parse_mode":"HTML","link_preview_options":{"is_disabled":true},"disable_notification":true,"protect_content":true}`},
		{"values of the call", "sendMessage", []Option{all}, func(c *Client) error {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi", ParseMode: Ptr(""),
				LinkPreviewOptions: &models.LinkPreviewOptions{}, DisableNotification: Ptr(false), ProtectContent: Ptr(false)})
			return err
		}, `{"chat_id":7,"text":"hi","parse_mode":"","link_preview_options":{},"disable_notification":false,"protect_content":false}`},
		{"entities instead of a parse mode", "sendMessage", []Option{WithDefaults(Defaults{ParseMode: ParseModeHTML})}, func(c *Client) error {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi", Entities: []models.MessageEntity{{Type: "bold", Length: 2}}})
			return err
		}, `{"chat_id":7,"text":"hi","entities":[{"type":"bold","offset":0,"length":2}]}`},
		{"a caption", "sendPhoto", []Option{WithDefaults(Defaults{ParseMode: ParseModeMarkdown})}, func(c *Client) error {
			_, err := c.SendPhoto(ctx, SendPhotoParams{ChatID: chat, Photo: models.FileID("p"), Caption: Ptr("*hi*")})
			return err
		}, `{"chat_id":7,"photo":"p","caption":"*hi*","parse_mode":"MarkdownV2"}`},
		{"caption entities instead of a parse mode", "copyMessage", []Option{all}, func(c *Client) error {
			_, err := c.CopyMessage(ctx, CopyMessageParams{ChatID: chat, FromChatID: chat, MessageID: 1, CaptionEntities: []models.MessageEntity{{Type: "bold", Length: 1}}})
			return err
		}, `{"chat_id":7,"from_chat_id":7,"message_id":1,"caption_entities":[{"type":"bold","offset":0,"length":1}],"disable_notification":true,"protect_content":true}`},
		{"an edit", "editMessageText", []Option{all}, func(c *Client) error {
			_, err := c.EditMessageText(ctx, EditMessageTextParams{ChatID: chat, MessageID: Ptr(int64(1)), Text: Ptr("hi")})
			return err
		}, `{"chat_id":7,"message_id":1,"text":"hi","parse_mode":"HTML","link_preview_options":{"is_disabled":true}}`},
		{"a method without these parameters", "sendChatAction", []Option{all}, func(c *Client) error {
			return c.SendChatAction(ctx, SendChatActionParams{ChatID: chat, Action: "typing"})
		}, `{"chat_id":7,"action":"typing"}`},
		{"Call", "sendMessage", []Option{all}, func(c *Client) error {
			return c.Call(ctx, "sendMessage", map[string]any{"chat_id": 7, "text": "hi"}, nil)
		}, `{"chat_id":7,"text":"hi"}`},
		{"zero defaults", "sendMessage", []Option{WithDefaults(Defaults{})}, func(c *Client) error {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi"})
			return err
		}, `{"chat_id":7,"text":"hi"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, sent := bodies(t, tt.opts...)
			if err := tt.call(c); err != nil {
				t.Fatal(err)
			}
			if got := sent[tt.method]; got != tt.want {
				t.Errorf("%s sent %s\nwant %s", tt.method, got, tt.want)
			}
		})
	}
}

func TestDefaultsAreCopied(t *testing.T) {
	lp := &models.LinkPreviewOptions{IsDisabled: Ptr(true)}
	c, sent := bodies(t, WithDefaults(Defaults{LinkPreviewOptions: lp}))
	lp.IsDisabled = Ptr(false)
	params := SendMessageParams{ChatID: models.ID(7), Text: "hi"}
	if _, err := c.SendMessage(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	if want := `{"chat_id":7,"text":"hi","link_preview_options":{"is_disabled":true}}`; sent["sendMessage"] != want {
		t.Errorf("sent %s, want %s: the defaults changed with the value given to WithDefaults", sent["sendMessage"], want)
	}
	if params.LinkPreviewOptions != nil {
		t.Errorf("the defaults changed the parameters of the caller")
	}
}
