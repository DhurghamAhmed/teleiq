package filter_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestShortcutFilters(t *testing.T) {
	from := `"from":{"id":7,"is_bot":false,"first_name":"Ann"},"chat_instance":"c"`
	button := `"message":{"message_id":1,"date":1,"chat":{"id":7,"type":"private"},"text":"hi"}`
	more := append(updates[:len(updates):len(updates)], []struct{ name, json string }{
		{"color button", `{"update_id":10,"callback_query":{"id":"q",` + from + `,` + button + `,"data":"color:red"}}`},
		{"size button", `{"update_id":11,"callback_query":{"id":"q",` + from + `,` + button + `,"data":"size:big"}}`},
		{"inline color button", `{"update_id":12,"callback_query":{"id":"q",` + from + `,"inline_message_id":"i","data":"color:"}}`},
		{"game button", `{"update_id":13,"callback_query":{"id":"q",` + from + `,` + button + `,"game_short_name":"g"}}`},
		{"unknown command", `{"update_id":14,"message":{"message_id":14,"date":1,` + private + `,"text":"/unknown"}}`},
		{"command for this bot", `{"update_id":15,"message":{"message_id":15,"date":1,` + private + `,"text":"/help@test_bot"}}`},
		{"photo with a caption", `{"update_id":16,"message":{"message_id":16,"date":1,` + private + `,"caption":"hi","photo":[{"file_id":"p","file_unique_id":"u","width":1,"height":1}]}}`},
		{"channel text", `{"update_id":17,"channel_post":{"message_id":17,"date":1,` + channel + `,"text":"hi"}}`},
		{"color text", `{"update_id":18,"message":{"message_id":18,"date":1,` + private + `,"text":"color:red"}}`},
		{"upper-case command", `{"update_id":19,"message":{"message_id":19,"date":1,` + private + `,"text":"/HELP"}}`},
		{"slash alone", `{"update_id":20,"message":{"message_id":20,"date":1,` + private + `,"text":"/"}}`},
		{"invalid command", `{"update_id":21,"message":{"message_id":21,"date":1,` + private + `,"text":"/say-hi"}}`},
		{"text with a command inside", `{"update_id":22,"message":{"message_id":22,"date":1,` + private + `,"text":"send /help"}}`},
	}...)

	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	b := bot.New(client)
	// Once Run handles an update, the bot knows its username, test_bot, so a command addressed to
	// it matches.
	stop := teleiqtest.Start(t, b.Run)
	srv.WaitHandled(t, srv.Push(teleiqtest.MessageUpdate(7, "hi")))
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()

	commands := []string{"group command", "unknown command", "command for this bot", "upper-case command"}
	texts := []string{"private text", "command for another bot", "color text", "slash alone", "invalid command", "text with a command inside"}
	tests := []struct {
		name   string
		filter bot.Filter
		want   []string
	}{
		{"any command", filter.AnyCommand(), commands},
		{"any text", filter.AnyText(), texts},
		{"any text or any command", filter.Or(filter.AnyText(), filter.AnyCommand()), []string{"private text", "group command", "command for another bot", "unknown command", "command for this bot", "color text", "upper-case command", "slash alone", "invalid command", "text with a command inside"}},
		{"any text and any command", filter.And(filter.AnyText(), filter.AnyCommand()), nil},
		{"callback prefix", filter.CallbackPrefix("color:"), []string{"color button", "inline color button"}},
		{"another callback prefix", filter.CallbackPrefix("size"), []string{"size button"}},
		{"empty callback prefix", filter.CallbackPrefix(""), []string{"color button", "size button", "inline color button"}},
		{"callback prefix that no data has", filter.CallbackPrefix("color:red:"), nil},
		{"callback prefix inside the data", filter.CallbackPrefix("red"), nil},
		{"any command but a known one", filter.And(filter.AnyCommand(), filter.Not(filter.Command("start"))), commands[1:]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, u := range more {
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
