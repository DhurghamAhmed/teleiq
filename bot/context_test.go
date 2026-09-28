package bot

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// newContext returns a Context for u whose bot is "test_bot", and the bodies of the requests it sends.
func newContext(t *testing.T, u string) (*Context, *[]string) {
	t.Helper()
	var update models.Update
	if err := json.Unmarshal([]byte(u), &update); err != nil {
		t.Fatalf("update %s: %v", u, err)
	}
	var sent []string
	record := teleiq.TransportFunc(func(_ context.Context, req *teleiq.Request) (*teleiq.Response, error) {
		body, _ := io.ReadAll(req.Body)
		sent = append(sent, req.Method+" "+string(body))
		return &teleiq.Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"message_id":99,"date":1,"chat":{"id":1,"type":"private"}}}`)}, nil
	})
	client, err := teleiq.NewClient(testToken, teleiq.WithTransport(record))
	if err != nil {
		t.Fatal(err)
	}
	b := New(client)
	b.me = &models.User{ID: 123456, IsBot: true, FirstName: "Test Bot", Username: teleiq.Ptr("test_bot")}
	return &Context{update: &update, bot: b}, &sent
}

const (
	ann     = `{"id":7,"is_bot":false,"first_name":"Ann"}`
	private = `{"id":7,"type":"private"}`
	forum   = `{"id":-100,"type":"supergroup","is_forum":true}`
)

func TestContextAccessors(t *testing.T) {
	tests := []struct {
		name       string
		update     string
		wantMsg    int64 // the message_id of Message, or 0 for nil
		wantChat   int64
		wantSender int64
	}{
		{"message", `{"update_id":1,"message":{"message_id":5,"date":1,"chat":` + private + `,"from":` + ann + `}}`, 5, 7, 7},
		{"edited message", `{"update_id":1,"edited_message":{"message_id":6,"date":1,"chat":` + private + `,"from":` + ann + `}}`, 6, 7, 7},
		{"channel post", `{"update_id":1,"channel_post":{"message_id":8,"date":1,"chat":{"id":-5,"type":"channel"}}}`, 8, -5, 0},
		{"callback on a message", `{"update_id":1,"callback_query":{"id":"q","from":` + ann + `,"chat_instance":"c","message":{"message_id":9,"date":1,"chat":` + private + `}}}`, 9, 7, 7},
		{"callback on an inaccessible message", `{"update_id":1,"callback_query":{"id":"q","from":` + ann + `,"chat_instance":"c","message":{"message_id":9,"date":0,"chat":` + private + `}}}`, 0, 7, 7},
		{"inline query", `{"update_id":1,"inline_query":{"id":"i","from":` + ann + `,"query":"","offset":""}}`, 0, 0, 7},
		{"poll", `{"update_id":1,"poll":{"id":"p","question":"q","options":[],"total_voter_count":0,"is_closed":false,"is_anonymous":true,"type":"regular","allows_multiple_answers":false}}`, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newContext(t, tt.update)
			var msg, chat, sender int64
			if m := c.Message(); m != nil {
				msg = m.MessageID
			}
			if ch := c.Chat(); ch != nil {
				chat = ch.ID
			}
			if s := c.Sender(); s != nil {
				sender = s.ID
			}
			if msg != tt.wantMsg || chat != tt.wantChat || sender != tt.wantSender {
				t.Errorf("Message, Chat, Sender = %d, %d, %d; want %d, %d, %d", msg, chat, sender, tt.wantMsg, tt.wantChat, tt.wantSender)
			}
		})
	}
}

func TestContextCommand(t *testing.T) {
	msg := func(text string) string {
		return `{"update_id":1,"message":{"message_id":1,"date":1,"chat":` + private + `,"text":` + jsonString(text) + `}}`
	}
	tests := []struct {
		name, update, wantCommand, wantArgs string
	}{
		{"command", msg("/start"), "start", ""},
		{"arguments", msg("/start hello there"), "start", "hello there"},
		{"our mention", msg("/Start@Test_Bot 42"), "Start", "42"},
		{"another bot", msg("/start@other_bot 42"), "", ""},
		{"plain text", msg("hello"), "", ""},
		{"no text", `{"update_id":1,"message":{"message_id":1,"date":1,"chat":` + private + `}}`, "", ""},
		{"edited message", `{"update_id":1,"edited_message":{"message_id":1,"date":1,"chat":` + private + `,"text":"/start"}}`, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newContext(t, tt.update)
			if cmd, args := c.Command(), c.Args(); cmd != tt.wantCommand || args != tt.wantArgs {
				t.Errorf("Command(), Args() = %q, %q; want %q, %q", cmd, args, tt.wantCommand, tt.wantArgs)
			}
		})
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestContextReply(t *testing.T) {
	tests := []struct {
		name      string
		update    string
		opts      []SendOption
		wantSent  string
		wantInErr string
	}{
		{
			name:     "private chat",
			update:   `{"update_id":1,"message":{"message_id":5,"date":1,"chat":` + private + `,"text":"hi"}}`,
			wantSent: `sendMessage {"chat_id":7,"text":"Hello!"}`,
		},
		{
			name:     "forum topic",
			update:   `{"update_id":1,"message":{"message_id":5,"message_thread_id":3,"is_topic_message":true,"date":1,"chat":` + forum + `}}`,
			wantSent: `sendMessage {"chat_id":-100,"message_thread_id":3,"text":"Hello!"}`,
		},
		{
			name:     "reply thread outside a forum",
			update:   `{"update_id":1,"message":{"message_id":5,"message_thread_id":3,"date":1,"chat":{"id":-200,"type":"supergroup"}}}`,
			wantSent: `sendMessage {"chat_id":-200,"text":"Hello!"}`,
		},
		{
			name:     "business chat",
			update:   `{"update_id":1,"business_message":{"message_id":5,"business_connection_id":"bc1","date":1,"chat":` + private + `}}`,
			wantSent: `sendMessage {"business_connection_id":"bc1","chat_id":7,"text":"Hello!"}`,
		},
		{
			name:     "direct messages topic",
			update:   `{"update_id":1,"message":{"message_id":5,"direct_messages_topic":{"topic_id":11},"date":1,"chat":{"id":-300,"type":"supergroup"}}}`,
			wantSent: `sendMessage {"chat_id":-300,"direct_messages_topic_id":11,"text":"Hello!"}`,
		},
		{
			name:     "callback query",
			update:   `{"update_id":1,"callback_query":{"id":"q","from":` + ann + `,"chat_instance":"c","message":{"message_id":9,"date":1,"chat":` + private + `}}}`,
			wantSent: `sendMessage {"chat_id":7,"text":"Hello!"}`,
		},
		{
			name:   "options",
			update: `{"update_id":1,"message":{"message_id":5,"date":1,"chat":` + private + `}}`,
			opts: []SendOption{nil, func(p *teleiq.SendMessageParams) {
				p.ParseMode = teleiq.Ptr("HTML")
				p.ReplyParameters = &models.ReplyParameters{MessageID: teleiq.Ptr(int64(5))}
			}},
			wantSent: `sendMessage {"chat_id":7,"text":"Hello!","parse_mode":"HTML","reply_parameters":{"message_id":5}}`,
		},
		{
			name:      "no chat",
			update:    `{"update_id":1,"inline_query":{"id":"i","from":` + ann + `,"query":"","offset":""}}`,
			wantInErr: "has no chat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, sent := newContext(t, tt.update)
			msg, err := c.Reply(context.Background(), "Hello!", tt.opts...)
			if tt.wantInErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantInErr) || len(*sent) != 0 {
					t.Fatalf("Reply() = %v after %d requests, want an error mentioning %q and none", err, len(*sent), tt.wantInErr)
				}
				return
			}
			if err != nil || msg.MessageID != 99 {
				t.Fatalf("Reply() = %+v, %v", msg, err)
			}
			if len(*sent) != 1 || (*sent)[0] != tt.wantSent {
				t.Errorf("sent %q, want %q", *sent, tt.wantSent)
			}
		})
	}
}

func TestNewContextWithoutUpdate(t *testing.T) {
	b, _ := newTestBot(t, newFakeTelegram())
	c := b.NewContext(nil)
	if c.Update() == nil || c.Message() != nil || c.Chat() != nil || c.Sender() != nil || c.Command() != "" || c.Args() != "" {
		t.Errorf("a Context without an update has content: %+v", c.Update())
	}
	if _, err := c.Reply(context.Background(), "hi"); err == nil {
		t.Error("Reply() succeeded without a chat")
	}
}
