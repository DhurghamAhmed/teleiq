package schema

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParsePage(t *testing.T) {
	page, err := os.ReadFile("testdata/page.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(bytes.NewReader(page))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := func(name string, types []string, required bool, desc string) Field {
		return Field{Name: name, Types: types, Required: required, Description: desc}
	}
	const methods, types = "Available methods", "Available types"
	want := &API{
		Version:     "9.5",
		ReleaseDate: "2026-03-01",
		Methods: []Method{
			{Name: "getUpdates", Section: "Getting updates", Description: "Use this method to receive incoming updates. Returns an Array of Update objects.", Params: []Field{}, Returns: []string{"Array of Update"}},
			{Name: "getMe", Section: methods, Description: "A simple method for testing your bot's authentication token. Requires no parameters. Returns basic information about the bot in form of a User object.", Params: []Field{}, Returns: []string{"User"}},
			{Name: "sendMessage", Section: methods, Description: "Use this method to send text messages. On success, the sent Message is returned.", Params: []Field{
				f("chat_id", []string{"Integer", "String"}, true, "Unique identifier for the target chat"),
				f("photos", []string{"Array of InputFile", "Array of String", "Array of Integer"}, false, "A list"),
				f("entities", []string{"Array of MessageEntity"}, false, "Special entities"),
			}, Returns: []string{"Message"}},
			{Name: "editMessageText", Section: methods, Description: "Use this method to edit text messages. On success, if the edited message is not an inline message, the edited Message is returned, otherwise True is returned.", Params: []Field{}, Returns: []string{"Message", "True"}},
			{Name: "getChatMemberCount", Section: methods, Description: "Use this method to get the number of members in a chat. Returns Int on success.", Params: []Field{}, Returns: []string{"Integer"}},
		},
		Types: []Type{
			{Name: "Update", Section: "Getting updates", Description: "This object represents an incoming update. At most one of the optional fields can be present in any given update.", Fields: []Field{
				f("update_id", []string{"Integer"}, true, "The update's unique identifier."),
				f("message", []string{"Message"}, false, "New incoming message."),
			}},
			{Name: "User", Section: types, Description: "This object represents a Telegram user or bot.", Fields: []Field{
				f("id", []string{"Integer"}, true, "Unique identifier for this user or bot."),
				f("is_premium", []string{"True"}, false, "True, if this user is a Telegram Premium user"),
			}},
			{Name: "Message", Section: types, Description: "This object represents a message.", Fields: []Field{
				f("message_id", []string{"Integer"}, true, "Unique message identifier"),
				f("entities", []string{"Array of MessageEntity"}, false, "Special entities in the text"),
			}},
			{Name: "MessageEntity", Section: types, Description: "This object represents one special entity in a text message.", Fields: []Field{
				f("type", []string{"String"}, true, "Type of the entity"),
			}},
			{Name: "MessageOrigin", Section: types, Description: "This object describes the origin of a message. It can be one of", Fields: []Field{},
				Members: []string{"MessageOriginUser", "MessageOriginChat"}, Discriminator: "type"},
			{Name: "MessageOriginUser", Section: types, Description: "The message was originally sent by a known user.", Fields: []Field{
				{Name: "type", Types: []string{"String"}, Required: true, Const: "user", Description: "Type of the message origin, always “user”"},
				f("sender_user", []string{"User"}, true, "User that sent the message originally"),
			}},
			{Name: "MessageOriginChat", Section: types, Description: "The message was originally sent on behalf of a chat.", Fields: []Field{
				{Name: "type", Types: []string{"String"}, Required: true, Const: "chat", Description: "Type of the message origin, must be chat"},
			}},
			{Name: "RichText", Section: types, Description: "This object represents a rich formatted text. Currently, it can be either a String for plain text, an Array of RichText, or any of the following types:", Fields: []Field{},
				Members: []string{"String", "Array of RichText", "RichTextBold"}, Discriminator: "type"},
			{Name: "RichTextBold", Section: types, Description: "A bold rich text.", Fields: []Field{
				{Name: "type", Types: []string{"String"}, Required: true, Const: "bold", Description: "Type of the rich text, always “bold”"},
				f("text", []string{"RichText"}, true, "The text"),
			}},
			{Name: "InputMessageContent", Section: types, Description: "This object represents the content of a message to be sent. It can be one of", Fields: []Field{},
				Members: []string{"InputTextMessageContent"}},
			{Name: "InputTextMessageContent", Section: types, Description: "Represents the content of a text message.", Fields: []Field{
				f("message_text", []string{"String"}, true, "Text of the message to be sent, 1-4096 characters"),
			}},
			{Name: "CallbackGame", Section: types, Description: "A placeholder, currently holds no information.", Fields: []Field{}},
			{Name: "InputFile", Section: types, Description: "This object represents the contents of a file to be uploaded.", Fields: []Field{}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		var g, w bytes.Buffer
		_ = Encode(&g, got)
		_ = Encode(&w, want)
		t.Errorf("Parse() mismatch\n got: %s\nwant: %s", g.String(), w.String())
	}
}

func TestParseRejectsUnexpectedPages(t *testing.T) {
	const release = `<h3>Recent changes</h3><h4>March 1, 2026</h4><p>Bot API 9.5</p>`
	const typ = `<h3>Available types</h3><h4>User</h4><p>A user.</p>`
	const method = `<h3>Available methods</h3><h4>getMe</h4><p>Returns True on success.</p>`
	tests := []struct {
		name string
		page string
		want string
	}{
		{"no release", typ + method, "no release found"},
		{"unreadable release date", `<h3>Recent changes</h3><h4>Someday</h4><p>Bot API 9.5</p>` + typ + method, "cannot read the latest release"},
		{"no methods", release + typ, "found 0 methods"},
		{"no types", release + method, "and 0 types"},
		{"method without return type", release + typ + `<h3>M</h3><h4>getMe</h4><p>Does something.</p>`, "no return type"},
		{"unexpected parameter header", release + typ + method + `<table><tr><th>Param</th><th>Type</th></tr></table>`, "unexpected table header"},
		{"unexpected required value", release + typ + method + `<table><tr><th>Parameter</th><th>Type</th><th>Required</th><th>Description</th></tr><tr><td>a</td><td>String</td><td>Maybe</td><td>x</td></tr></table>`, "unexpected parameter row"},
		{"unexpected field header", release + typ + `<table><tr><th>Name</th></tr></table>` + method, "unexpected table header"},
		{"unknown union member", release + typ + `<ul><li><a href="#nosuchtype">NoSuchType</a></li></ul>` + method, "not a known type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.page))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse() error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestTypeExpr(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"Integer", []string{"Integer"}},
		{"Integer or String", []string{"Integer", "String"}},
		{"InputFile or String", []string{"InputFile", "String"}},
		{"Array of MessageEntity", []string{"Array of MessageEntity"}},
		{"Array of Array of PhotoSize", []string{"Array of Array of PhotoSize"}},
		{"Array of InputMediaAudio, InputMediaDocument, InputMediaPhoto and InputMediaVideo", []string{"Array of InputMediaAudio", "Array of InputMediaDocument", "Array of InputMediaPhoto", "Array of InputMediaVideo"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := typeExpr(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("typeExpr(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
