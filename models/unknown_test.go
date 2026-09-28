package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDecodeUnion(t *testing.T) {
	user := User{ID: 1, FirstName: "Ann"}
	tests := []struct {
		name   string
		decode func([]byte) (any, error)
		in     string
		want   any
	}{
		{"origin user", anyOf(decodeMessageOrigin), `{"type":"user","date":5,"sender_user":{"id":1,"is_bot":false,"first_name":"Ann"}}`,
			&MessageOriginUser{Date: 5, SenderUser: user}},
		{"origin hidden user", anyOf(decodeMessageOrigin), `{"type":"hidden_user","date":5,"sender_user_name":"Bob"}`,
			&MessageOriginHiddenUser{Date: 5, SenderUserName: "Bob"}},
		{"new kind", anyOf(decodeMessageOrigin), `{"type":"future","date":5}`,
			&Unknown{Kind: "future", Raw: json.RawMessage(`{"type":"future","date":5}`)}},
		{"null", anyOf(decodeMessageOrigin), `null`, nil},
		{"absent", anyOf(decodeMessageOrigin), ``, nil},
		{"banned member", anyOf(decodeChatMember), `{"status":"kicked","user":{"id":1,"is_bot":false,"first_name":"Ann"},"until_date":0}`,
			&ChatMemberBanned{User: user}},
		{"member", anyOf(decodeChatMember), `{"status":"member","user":{"id":1,"is_bot":false,"first_name":"Ann"}}`,
			&ChatMemberMember{User: user}},
		{"inaccessible message", anyOf(decodeMaybeInaccessibleMessage), `{"chat":{"id":2,"type":"group"},"message_id":3,"date":0}`,
			&InaccessibleMessage{Chat: Chat{ID: 2, Type: "group"}, MessageID: 3}},
		{"accessible message", anyOf(decodeMaybeInaccessibleMessage), `{"chat":{"id":2,"type":"group"},"message_id":3,"date":9,"text":"hi"}`,
			&Message{Chat: Chat{ID: 2, Type: "group"}, MessageID: 3, Date: 9, Text: ptr("hi")}},
		{"rich text string", anyOf(decodeRichText), `"plain"`, RichTextString("plain")},
		{"rich text object", anyOf(decodeRichText), `{"type":"bold","text":"x"}`, &RichTextBold{Text: RichTextString("x")}},
		{"rich text array", anyOf(decodeRichText), `["a",{"type":"bold","text":["b","c"]}]`,
			RichTextArray{RichTextString("a"), &RichTextBold{Text: RichTextArray{RichTextString("b"), RichTextString("c")}}}},
		{"rich text new kind", anyOf(decodeRichText), `{"type":"sparkle"}`, &Unknown{Kind: "sparkle", Raw: json.RawMessage(`{"type":"sparkle"}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.decode([]byte(tt.in))
			if err != nil {
				t.Fatalf("decode(%s) error = %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decode(%s) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// anyOf lets decoders of different unions share one test table.
func anyOf[T any](decode func([]byte) (T, error)) func([]byte) (any, error) {
	return func(data []byte) (any, error) {
		v, err := decode(data)
		if reflect.ValueOf(&v).Elem().IsNil() {
			return nil, err
		}
		return v, err
	}
}

func TestDecodeUnionErrors(t *testing.T) {
	tests := []struct{ name, in string }{
		{"not an object", `[1]`},
		{"wrong field type", `{"type":"user","date":"yesterday"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if v, err := decodeMessageOrigin([]byte(tt.in)); v != nil || err == nil {
				t.Fatalf("decodeMessageOrigin(%s) = %v, %v; want an error", tt.in, v, err)
			}
		})
	}
}

func TestMessageWithUnionFields(t *testing.T) {
	in := `{"message_id":7,"date":1,"chat":{"id":1,"type":"private"},` +
		`"forward_origin":{"type":"channel","date":2,"chat":{"id":-100,"type":"channel"},"message_id":4},` +
		`"pinned_message":{"chat":{"id":1,"type":"private"},"message_id":3,"date":0}}`
	var m Message
	if err := json.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	origin, ok := m.ForwardOrigin.(*MessageOriginChannel)
	if !ok || origin.Chat.ID != -100 || origin.MessageID != 4 {
		t.Errorf("ForwardOrigin = %#v, want a *MessageOriginChannel", m.ForwardOrigin)
	}
	if _, ok := m.PinnedMessage.(*InaccessibleMessage); !ok {
		t.Errorf("PinnedMessage = %#v, want an *InaccessibleMessage", m.PinnedMessage)
	}

	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var again Message
	if err := json.Unmarshal(out, &again); err != nil || !reflect.DeepEqual(again, m) {
		t.Errorf("round trip = %#v, %v; want %#v", again, err, m)
	}
}

func TestConstantFields(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"discriminator", &ReactionTypeEmoji{Emoji: "👍"}, `{"type":"emoji","emoji":"👍"}`},
		{"discriminator only", BotCommandScopeDefault{}, `{"type":"default"}`},
		{"status", ChatMemberBanned{User: User{ID: 1, FirstName: "A"}}, `{"status":"kicked","user":{"id":1,"is_bot":false,"first_name":"A"},"until_date":0}`},
		{"required true", ForceReply{Selective: ptr(true)}, `{"force_reply":true,"selective":true}`},
		{"required true only", ReplyKeyboardRemove{}, `{"remove_keyboard":true}`},
		{"in a union field", struct {
			M ReplyMarkup `json:"reply_markup"`
		}{&ForceReply{}}, `{"reply_markup":{"force_reply":true}}`},
		{"unknown kept as received", []ReactionType{&Unknown{Kind: "x", Raw: json.RawMessage(`{"type":"x","n":1}`)}}, `[{"type":"x","n":1}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil || string(got) != tt.want {
				t.Fatalf("Marshal() = %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestUnknownWithoutRaw(t *testing.T) {
	if _, err := json.Marshal(Unknown{Kind: "x"}); err == nil {
		t.Fatal("Marshal(Unknown without Raw) succeeded, want an error")
	}
}
