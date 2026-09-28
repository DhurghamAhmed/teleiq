package bot

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// destinationParams are the parameters that say where a message goes.
var destinationParams = []string{"chat_id", "message_thread_id", "business_connection_id", "direct_messages_topic_id"}

func TestContextSendDestination(t *testing.T) {
	const (
		topicMessage   = `{"update_id":1,"message":{"message_id":5,"message_thread_id":3,"is_topic_message":true,"date":1,"chat":` + forum + `}}`
		everywhere     = `{"update_id":1,"business_message":{"message_id":5,"message_thread_id":3,"is_topic_message":true,"business_connection_id":"bc1","direct_messages_topic":{"topic_id":11},"date":1,"chat":` + forum + `}}`
		businessUpdate = `{"update_id":1,"business_message":{"message_id":5,"business_connection_id":"bc1","date":1,"chat":` + private + `}}`
	)
	photo := func(ctx context.Context, c *Context) error {
		_, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{Photo: models.FileFromBytes("a.png", []byte("png"))})
		return err
	}
	tests := []struct {
		name   string
		update string
		send   func(ctx context.Context, c *Context) error
		method string
		want   map[string]any // the parameters of destinationParams that the request carries
	}{
		{
			name:   "forum topic",
			update: topicMessage,
			send:   photo,
			method: "sendPhoto",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 3.0},
		},
		{
			name:   "reply thread outside a forum topic",
			update: `{"update_id":1,"message":{"message_id":5,"message_thread_id":3,"date":1,"chat":{"id":-200,"type":"supergroup"}}}`,
			send:   photo,
			method: "sendPhoto",
			want:   map[string]any{"chat_id": -200.0},
		},
		{
			name:   "business connection",
			update: businessUpdate,
			send:   photo,
			method: "sendPhoto",
			want:   map[string]any{"chat_id": 7.0, "business_connection_id": "bc1"},
		},
		{
			name:   "direct messages topic",
			update: `{"update_id":1,"message":{"message_id":5,"direct_messages_topic":{"topic_id":11},"date":1,"chat":{"id":-300,"type":"supergroup"}}}`,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendDocument(ctx, teleiq.SendDocumentParams{Document: models.FileID("doc")})
				return err
			},
			method: "sendDocument",
			want:   map[string]any{"chat_id": -300.0, "direct_messages_topic_id": 11.0},
		},
		{
			name:   "all of them",
			update: everywhere,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendMessage(ctx, teleiq.SendMessageParams{Text: "hi"})
				return err
			},
			method: "sendMessage",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 3.0, "business_connection_id": "bc1", "direct_messages_topic_id": 11.0},
		},
		{
			name:   "a method without a direct messages topic",
			update: everywhere,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendGame(ctx, teleiq.SendGameParams{GameShortName: "g"})
				return err
			},
			method: "sendGame",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 3.0, "business_connection_id": "bc1"},
		},
		{
			name:   "a method without a business connection",
			update: everywhere,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendInvoice(ctx, teleiq.SendInvoiceParams{Title: "t", Description: "d", Payload: "p", Currency: "XTR"})
				return err
			},
			method: "sendInvoice",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 3.0, "direct_messages_topic_id": 11.0},
		},
		{
			name:   "a required business connection",
			update: businessUpdate,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendChecklist(ctx, teleiq.SendChecklistParams{Checklist: models.InputChecklist{Title: "todo"}})
				return err
			},
			method: "sendChecklist",
			want:   map[string]any{"chat_id": 7.0, "business_connection_id": "bc1"},
		},
		{
			name:   "a required business connection that p sets",
			update: businessUpdate,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendChecklist(ctx, teleiq.SendChecklistParams{BusinessConnectionID: "mine", Checklist: models.InputChecklist{Title: "todo"}})
				return err
			},
			method: "sendChecklist",
			want:   map[string]any{"chat_id": 7.0, "business_connection_id": "mine"},
		},
		{
			name:   "values that p sets",
			update: everywhere,
			send: func(ctx context.Context, c *Context) error {
				_, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{
					ChatID: models.ID(999), Photo: models.FileID("p"), MessageThreadID: teleiq.Ptr(int64(4)),
					BusinessConnectionID: teleiq.Ptr("bc2"), DirectMessagesTopicID: teleiq.Ptr(int64(12)),
				})
				return err
			},
			method: "sendPhoto",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 4.0, "business_connection_id": "bc2", "direct_messages_topic_id": 12.0},
		},
		{
			name:   "callback query on a message",
			update: `{"update_id":1,"callback_query":{"id":"q","from":` + ann + `,"chat_instance":"c","message":{"message_id":9,"message_thread_id":3,"is_topic_message":true,"date":1,"chat":` + forum + `}}}`,
			send: func(ctx context.Context, c *Context) error {
				return c.SendChatAction(ctx, teleiq.SendChatActionParams{Action: "typing"})
			},
			method: "sendChatAction",
			want:   map[string]any{"chat_id": -100.0, "message_thread_id": 3.0},
		},
		{
			name:   "callback query on an inaccessible message",
			update: `{"update_id":1,"callback_query":{"id":"q","from":` + ann + `,"chat_instance":"c","message":{"message_id":9,"date":0,"chat":` + forum + `}}}`,
			send: func(ctx context.Context, c *Context) error {
				msgs, err := c.SendMediaGroup(ctx, teleiq.SendMediaGroupParams{Media: []models.InputMediaGroupItem{
					&models.InputMediaPhoto{Media: models.FileID("a")}, &models.InputMediaPhoto{Media: models.FileID("b")},
				}})
				if err == nil && len(msgs) != 2 {
					t.Errorf("SendMediaGroup() = %d messages, want 2", len(msgs))
				}
				return err
			},
			method: "sendMediaGroup",
			want:   map[string]any{"chat_id": -100.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := teleiqtest.NewServer()
			defer srv.Close()
			c := newServerContext(t, srv, tt.update)

			if err := tt.send(context.Background(), c); err != nil {
				t.Fatalf("send: %v", err)
			}

			reqs := srv.Requests("")
			if len(reqs) != 1 || reqs[0].Method != tt.method {
				t.Fatalf("requests = %+v, want one to %s", reqs, tt.method)
			}
			got := map[string]any{}
			for _, name := range destinationParams {
				if v, ok := reqs[0].Params[name]; ok {
					got[name] = v
				}
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("%s was sent to %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestContextSendWithoutChat(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	c := newServerContext(t, srv, `{"update_id":1,"inline_query":{"id":"i","from":`+ann+`,"query":"","offset":""}}`)
	ctx := context.Background()

	msg, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{ChatID: models.ID(7), Photo: models.FileID("p")})
	if msg != nil || err == nil || err.Error() != "bot: SendPhoto: the update has no chat" {
		t.Errorf("SendPhoto() = %v, %v; want the error for an update without a chat", msg, err)
	}
	if err := c.SendChatAction(ctx, teleiq.SendChatActionParams{Action: "typing"}); err == nil || err.Error() != "bot: SendChatAction: the update has no chat" {
		t.Errorf("SendChatAction() error = %v, want the error for an update without a chat", err)
	}
	if _, err := c.Reply(ctx, "hi"); err == nil || err.Error() != "bot: Reply: the update has no chat" {
		t.Errorf("Reply() error = %v, want the error for an update without a chat", err)
	}
	if reqs := srv.Requests(""); len(reqs) != 0 {
		t.Errorf("the bot made %d requests without a chat", len(reqs))
	}
}

// TestContextSendSignatures checks that each send method of Context takes and returns what the
// Client method of the same name does.
func TestContextSendSignatures(t *testing.T) {
	ctxType, clientType := reflect.TypeFor[*Context](), reflect.TypeFor[*teleiq.Client]()
	n := 0
	for i := range ctxType.NumMethod() {
		m := ctxType.Method(i)
		cm, ok := clientType.MethodByName(m.Name)
		if !ok || !strings.HasPrefix(m.Name, "Send") {
			continue
		}
		n++
		if got, want := m.Type.String(), strings.Replace(cm.Type.String(), "*teleiq.Client", "*bot.Context", 1); got != want {
			t.Errorf("Context.%s is %s, want %s", m.Name, got, want)
		}
	}
	if n != 22 {
		t.Errorf("Context has %d send methods of Client, want 22", n)
	}
}

// newServerContext returns a Context for the update u of a bot whose client calls srv.
func newServerContext(t *testing.T, srv *teleiqtest.Server, u string) *Context {
	t.Helper()
	var update models.Update
	if err := json.Unmarshal([]byte(u), &update); err != nil {
		t.Fatalf("update %s: %v", u, err)
	}
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	return New(client).NewContext(&update)
}
