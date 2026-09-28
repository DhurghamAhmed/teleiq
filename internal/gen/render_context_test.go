package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/internal/gen/schema"
)

func TestContextSendsFromSnapshot(t *testing.T) {
	m, err := newModel(loadAPI(t))
	if err != nil {
		t.Fatal(err)
	}
	sends, err := contextSends(m.methods)
	if err != nil {
		t.Fatalf("contextSends() error = %v", err)
	}
	got := map[string][]string{}
	for _, cs := range sends {
		var keys []string
		for _, slot := range cs.slots {
			keys = append(keys, slot.key)
		}
		got[cs.meth.api] = keys
	}
	all := []string{"thread", "business", "topic"}
	want := map[string][]string{
		"sendAnimation": all, "sendAudio": all, "sendChatAction": {"thread", "business"}, "sendChecklist": {"businessID"},
		"sendContact": all, "sendDice": all, "sendDocument": all, "sendGame": {"thread", "business"},
		"sendInvoice": {"thread", "topic"}, "sendLivePhoto": all, "sendLocation": all, "sendMediaGroup": all,
		"sendMessage": all, "sendPaidMedia": all, "sendPhoto": all, "sendPoll": {"thread", "business"},
		"sendRichMessage": all, "sendSticker": all, "sendVenue": all, "sendVideo": all, "sendVideoNote": all, "sendVoice": all,
	}
	for name, keys := range want {
		if !slices.Equal(got[name], keys) {
			t.Errorf("%s fills %v, want %v", name, got[name], keys)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is on Context but should not be", name)
		}
	}
	for _, name := range []string{"sendGift", "sendMessageDraft", "sendRichMessageDraft", "sendChatJoinRequestWebApp"} {
		if !slices.ContainsFunc(m.methods, func(meth *method) bool { return meth.api == name }) {
			t.Errorf("the snapshot has no %s to leave out", name)
		}
	}
}

// sendAPI returns an API whose methods return a Message and take params.
func sendAPI(methods map[string][]schema.Field) *schema.API {
	api := fakeAPI(schema.Type{Name: "Message"})
	for _, name := range slices.Sorted(maps.Keys(methods)) {
		api = withMethod(api, schema.Method{Name: name, Returns: []string{"Message"}, Params: methods[name]})
	}
	return api
}

func TestContextSendsRule(t *testing.T) {
	chat := schema.Field{Name: "chat_id", Types: []string{"Integer", "String"}, Required: true}
	optionalChat := schema.Field{Name: "chat_id", Types: []string{"Integer", "String"}}
	privateChat := schema.Field{Name: "chat_id", Types: []string{"Integer"}, Required: true}
	thread := schema.Field{Name: "message_thread_id", Types: []string{"Integer"}}
	business := schema.Field{Name: "business_connection_id", Types: []string{"String"}}
	tests := []struct {
		name    string
		params  []schema.Field
		want    []string // the keys it fills, or nil when it is left out
		wantErr string
	}{
		{name: "sendAll", params: []schema.Field{business, chat, thread, {Name: "direct_messages_topic_id", Types: []string{"Integer"}}}, want: []string{"thread", "business", "topic"}},
		{name: "sendChatOnly", params: []schema.Field{chat}, want: []string{}},
		{name: "sendRequiredBusiness", params: []schema.Field{{Name: "business_connection_id", Types: []string{"String"}, Required: true}, chat}, want: []string{"businessID"}},
		{name: "sendOptionalChat", params: []schema.Field{optionalChat}},
		{name: "sendPrivateChat", params: []schema.Field{privateChat, thread}},
		{name: "sendNoChat", params: []schema.Field{{Name: "user_id", Types: []string{"Integer"}, Required: true}}},
		{name: "sendNothing"},
		{name: "forwardMessage", params: []schema.Field{chat}},
		{name: "sendOddThread", params: []schema.Field{chat, {Name: "message_thread_id", Types: []string{"String"}}}, wantErr: "method sendOddThread: no rule to fill message_thread_id of the type *string"},
		{name: "sendRequiredThread", params: []schema.Field{chat, {Name: "message_thread_id", Types: []string{"Integer"}, Required: true}}, wantErr: "no rule to fill message_thread_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := newModel(sendAPI(map[string][]schema.Field{tt.name: tt.params}))
			if err != nil {
				t.Fatal(err)
			}
			sends, err := contextSends(m.methods)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("contextSends() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == nil {
				if len(sends) != 0 {
					t.Fatalf("contextSends() = %d methods, want none", len(sends))
				}
				return
			}
			if len(sends) != 1 {
				t.Fatalf("contextSends() = %d methods, want 1", len(sends))
			}
			keys := []string{}
			for _, slot := range sends[0].slots {
				keys = append(keys, slot.key)
			}
			if !slices.Equal(keys, tt.want) {
				t.Errorf("fills %v, want %v", keys, tt.want)
			}
		})
	}
}

func TestRenderContextSends(t *testing.T) {
	chat := schema.Field{Name: "chat_id", Types: []string{"Integer", "String"}, Required: true}
	api := sendAPI(map[string][]schema.Field{
		"sendNote":  {{Name: "business_connection_id", Types: []string{"String"}}, chat, {Name: "message_thread_id", Types: []string{"Integer"}}},
		"sendAlbum": {{Name: "business_connection_id", Types: []string{"String"}, Required: true}, chat},
	})
	api = withMethod(api, schema.Method{Name: "sendAlbums", Returns: []string{"Array of Message"}, Params: []schema.Field{chat}})
	api = withMethod(api, schema.Method{Name: "sendAction", Returns: []string{"True"}, Params: []schema.Field{chat, {Name: "direct_messages_topic_id", Types: []string{"Integer"}}}})
	m, err := newModel(api)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generate(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `// Code generated by teleiq-gen. DO NOT EDIT.

package bot

import (
	"context"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// SendAlbum calls Client.SendAlbum with p.ChatID set to the chat of the update. Like Reply, it
// answers in the same business connection as the message of the update, unless p sets it. It fails
// when the update has no chat.
func (c *Context) SendAlbum(ctx context.Context, p teleiq.SendAlbumParams) (*models.Message, error) {
	if err := c.fillDestination("SendAlbum", destinationFields{chat: &p.ChatID, businessID: &p.BusinessConnectionID}); err != nil {
		return nil, err
	}
	return c.bot.client.SendAlbum(ctx, p)
}

// SendNote calls Client.SendNote with p.ChatID set to the chat of the update. Like Reply, it
// answers in the same forum topic or business connection as the message of the update, unless p
// sets them. It fails when the update has no chat.
func (c *Context) SendNote(ctx context.Context, p teleiq.SendNoteParams) (*models.Message, error) {
	if err := c.fillDestination("SendNote", destinationFields{chat: &p.ChatID, thread: &p.MessageThreadID, business: &p.BusinessConnectionID}); err != nil {
		return nil, err
	}
	return c.bot.client.SendNote(ctx, p)
}

// SendAlbums calls Client.SendAlbums with p.ChatID set to the chat of the update. It fails when the
// update has no chat.
func (c *Context) SendAlbums(ctx context.Context, p teleiq.SendAlbumsParams) ([]models.Message, error) {
	if err := c.fillDestination("SendAlbums", destinationFields{chat: &p.ChatID}); err != nil {
		return nil, err
	}
	return c.bot.client.SendAlbums(ctx, p)
}

// SendAction calls Client.SendAction with p.ChatID set to the chat of the update. Like Reply, it
// answers in the same direct messages topic as the message of the update, unless p sets it. It
// fails when the update has no chat.
func (c *Context) SendAction(ctx context.Context, p teleiq.SendActionParams) error {
	if err := c.fillDestination("SendAction", destinationFields{chat: &p.ChatID, topic: &p.DirectMessagesTopicID}); err != nil {
		return err
	}
	return c.bot.client.SendAction(ctx, p)
}
`
	if got := string(files[contextFile]); got != want {
		t.Errorf("%s =\n%s\nwant\n%s", contextFile, got, want)
	}
}

func TestCheckContextNames(t *testing.T) {
	m, err := newModel(loadAPI(t))
	if err != nil {
		t.Fatal(err)
	}
	files, err := generate(m)
	if err != nil {
		t.Fatal(err)
	}
	generated := files[contextFile]
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{name: "no package bot"},
		{name: "other methods", files: map[string]string{"context.go": "package bot\n\nfunc (c *Context) Reply() {}\nfunc (b *Bot) SendPhoto() {}\nfunc SendPhoto() {}\n"}},
		{name: "the generated file itself", files: map[string]string{"context_send.gen.go": string(generated)}},
		{name: "a test", files: map[string]string{"x_test.go": "package bot_test\n\ntype Context struct{}\n\nfunc (Context) SendPhoto() {}\n"}},
		{
			name:    "a handwritten method",
			files:   map[string]string{"extra.go": "package bot\n\nfunc (c *Context) SendPhoto() {}\n"},
			wantErr: "Context.SendPhoto is written by hand but also generated in bot/context_send.gen.go",
		},
		{
			name:    "on a value receiver",
			files:   map[string]string{"extra.go": "package bot\n\nfunc (Context) SendDice() {}\n"},
			wantErr: "Context.SendDice is written by hand",
		},
		{name: "unparsable", files: map[string]string{"broken.go": "package bot\n\nfunc {"}, wantErr: "broken.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "bot")
			if tt.files != nil {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, src := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := checkContextNames(dir, generated)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("checkContextNames() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("checkContextNames() error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsHandwrittenClash(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bot"), 0o755); err != nil {
		t.Fatal(err)
	}
	clash := "package bot\n\nfunc (c *Context) SendMessage() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "bot", "send.go"), []byte(clash), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, check := range []bool{false, true} {
		if err := run(snapshotPath, dir, check); err == nil || !strings.Contains(err.Error(), "Context.SendMessage is written by hand") {
			t.Errorf("run(check = %v) error = %v, want the clash", check, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, contextFile)); !os.IsNotExist(err) {
		t.Errorf("run() wrote %s despite the clash: %v", contextFile, err)
	}
}
