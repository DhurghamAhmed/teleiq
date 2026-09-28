package filter_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
)

const (
	private = `"chat":{"id":7,"type":"private"},"from":{"id":7,"is_bot":false,"first_name":"Ann"}`
	group   = `"chat":{"id":-100,"type":"group"},"from":{"id":8,"is_bot":false,"first_name":"Bob"}`
	super   = `"chat":{"id":-200,"type":"supergroup"},"from":{"id":9,"is_bot":false,"first_name":"Cy"}`
	channel = `"chat":{"id":-300,"type":"channel"}`
)

// updates are the updates every filter is checked against, in this order.
var updates = []struct{ name, json string }{
	{"private text", `{"update_id":1,"message":{"message_id":1,"date":1,` + private + `,"text":"hi"}}`},
	{"group command", `{"update_id":2,"message":{"message_id":2,"date":1,` + group + `,"text":"/start now"}}`},
	{"supergroup photo", `{"update_id":3,"message":{"message_id":3,"date":1,` + super + `,"photo":[{"file_id":"p","file_unique_id":"u","width":1,"height":1}]}}`},
	{"channel document", `{"update_id":4,"channel_post":{"message_id":4,"date":1,` + channel + `,"document":{"file_id":"d","file_unique_id":"u"}}}`},
	{"private document", `{"update_id":5,"message":{"message_id":5,"date":1,` + private + `,"document":{"file_id":"d","file_unique_id":"u"}}}`},
	{"edited text", `{"update_id":6,"edited_message":{"message_id":1,"date":1,` + private + `,"text":"hi"}}`},
	{"private callback", `{"update_id":7,"callback_query":{"id":"q","chat_instance":"c","from":{"id":7,"is_bot":false,"first_name":"Ann"},"message":{"message_id":1,"date":1,"chat":{"id":7,"type":"private"},"text":"hi"}}}`},
	{"inline query", `{"update_id":8,"inline_query":{"id":"i","from":{"id":10,"is_bot":false,"first_name":"Di"},"query":"","offset":""}}`},
	{"command for another bot", `{"update_id":9,"message":{"message_id":9,"date":1,` + private + `,"text":"/start@other_bot"}}`},
}

func TestFilters(t *testing.T) {
	client, err := teleiq.NewClient("123:test")
	if err != nil {
		t.Fatal(err)
	}
	b := bot.New(client)
	var all []string
	for _, u := range updates {
		all = append(all, u.name)
	}
	tests := []struct {
		name   string
		filter bot.Filter
		want   []string
	}{
		{"command", filter.Command("start"), []string{"group command"}},
		{"command with a slash, another case, several names", filter.Command("/Start", "help"), []string{"group command"}},
		{"invalid command name", filter.Command("say hi"), nil},
		{"text", filter.Text("hi"), []string{"private text"}},
		{"photo", filter.Photo(), []string{"supergroup photo"}},
		{"document", filter.Document(), []string{"private document"}},
		{"private", filter.Private(), []string{"private text", "private document", "edited text", "private callback", "command for another bot"}},
		{"group", filter.Group(), []string{"group command", "supergroup photo"}},
		{"channel", filter.Channel(), []string{"channel document"}},
		{"chat ID", filter.ChatID(-100, 7), []string{"private text", "group command", "private document", "edited text", "private callback", "command for another bot"}},
		{"user ID", filter.UserID(9, 10), []string{"supergroup photo", "inline query"}},
		{"and", filter.And(filter.Private(), filter.Text("hi")), []string{"private text"}},
		{"and of nothing", filter.And(), all},
		{"and with nil", filter.And(filter.Private(), nil), nil},
		{"or", filter.Or(filter.Photo(), filter.Command("start")), []string{"group command", "supergroup photo"}},
		{"or of nothing", filter.Or(), nil},
		{"or with nil", filter.Or(nil, filter.Photo()), []string{"supergroup photo"}},
		{"not", filter.Not(filter.Private()), []string{"group command", "supergroup photo", "channel document", "inline query"}},
		{"not nil", filter.Not(nil), all},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, u := range updates {
				var update models.Update
				if err := json.Unmarshal([]byte(u.json), &update); err != nil {
					t.Fatalf("%s: %v", u.name, err)
				}
				if tt.filter.Match(b.NewContext(&update)) {
					got = append(got, u.name)
				}
			}
			if strings.Join(got, ", ") != strings.Join(tt.want, ", ") {
				t.Errorf("matched [%s], want [%s]", strings.Join(got, ", "), strings.Join(tt.want, ", "))
			}
		})
	}
}
