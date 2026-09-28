package filter_test

import (
	"regexp"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
)

func newTestBot(t *testing.T) *bot.Bot {
	t.Helper()
	client, err := teleiq.NewClient("123:test")
	if err != nil {
		t.Fatal(err)
	}
	return bot.New(client)
}

// newMessage returns a message from a user in a group, changed by set when it is not nil.
func newMessage(set func(m *models.Message)) *models.Message {
	m := &models.Message{MessageID: 1, Date: 1, Chat: models.Chat{ID: -100, Type: "group"}, From: &models.User{ID: 7, FirstName: "Ann"}}
	if set != nil {
		set(m)
	}
	return m
}

// TestMessageFilters checks that each filter about the content of a message matches a new message
// with its field, and no message without it, edit, channel post, business message or button press.
func TestMessageFilters(t *testing.T) {
	b := newTestBot(t)
	text := func(m *models.Message) { m.Text = teleiq.Ptr("hi") }
	tests := []struct {
		name   string
		filter bot.Filter
		set    func(m *models.Message)
	}{
		{"photo", filter.Photo(), func(m *models.Message) { m.Photo = []models.PhotoSize{{FileID: "p"}} }},
		{"live photo", filter.LivePhoto(), func(m *models.Message) { m.LivePhoto = &models.LivePhoto{} }},
		{"document", filter.Document(), func(m *models.Message) { m.Document = &models.Document{} }},
		{"animation", filter.Animation(), func(m *models.Message) { m.Animation = &models.Animation{} }},
		{"audio", filter.Audio(), func(m *models.Message) { m.Audio = &models.Audio{} }},
		{"video", filter.Video(), func(m *models.Message) { m.Video = &models.Video{} }},
		{"voice", filter.Voice(), func(m *models.Message) { m.Voice = &models.Voice{} }},
		{"video note", filter.VideoNote(), func(m *models.Message) { m.VideoNote = &models.VideoNote{} }},
		{"sticker", filter.Sticker(), func(m *models.Message) { m.Sticker = &models.Sticker{} }},
		{"media", filter.Media(), func(m *models.Message) { m.Voice = &models.Voice{} }},
		{"media group", filter.MediaGroup(), func(m *models.Message) { m.MediaGroupID = teleiq.Ptr("g") }},
		{"caption", filter.Caption(), func(m *models.Message) { m.Caption = teleiq.Ptr("c") }},
		{"spoiler", filter.Spoiler(), func(m *models.Message) { m.HasMediaSpoiler = true }},
		{"contact", filter.Contact(), func(m *models.Message) { m.Contact = &models.Contact{} }},
		{"location", filter.Location(), func(m *models.Message) { m.Location = &models.Location{} }},
		{"venue", filter.Venue(), func(m *models.Message) { m.Venue = &models.Venue{} }},
		{"poll", filter.Poll(), func(m *models.Message) { m.Poll = &models.Poll{} }},
		{"dice", filter.Dice(), func(m *models.Message) { m.Dice = &models.Dice{} }},
		{"game", filter.Game(), func(m *models.Message) { m.Game = &models.Game{} }},
		{"reply", filter.Reply(), func(m *models.Message) { m.ReplyToMessage = &models.Message{} }},
		{"forwarded", filter.Forwarded(), func(m *models.Message) { m.ForwardOrigin = &models.MessageOriginUser{} }},
		{"via bot", filter.ViaBot(), func(m *models.Message) { m.ViaBot = &models.User{IsBot: true} }},
		{"from a bot", filter.FromBot(), func(m *models.Message) { m.From.IsBot = true }},
		{"automatic forward", filter.AutomaticForward(), func(m *models.Message) { m.IsAutomaticForward = true }},
		{"from offline", filter.FromOffline(), func(m *models.Message) { m.IsFromOffline = true }},
		{"inline keyboard", filter.InlineKeyboard(), func(m *models.Message) { m.ReplyMarkup = &models.InlineKeyboardMarkup{} }},
		{"regex", filter.Regex(regexp.MustCompile(`^h`)), text},
		{"message", filter.Message(func(m *models.Message) bool { return m.Text != nil }), text},
		{"new chat members", filter.NewChatMembers(), func(m *models.Message) { m.NewChatMembers = []models.User{{ID: 8}} }},
		{"left chat member", filter.LeftChatMember(), func(m *models.Message) { m.LeftChatMember = &models.User{ID: 8} }},
		{"new chat title", filter.NewChatTitle(), func(m *models.Message) { m.NewChatTitle = teleiq.Ptr("t") }},
		{"new chat photo", filter.NewChatPhoto(), func(m *models.Message) { m.NewChatPhoto = []models.PhotoSize{{}} }},
		{"delete chat photo", filter.DeleteChatPhoto(), func(m *models.Message) { m.DeleteChatPhoto = true }},
		{"group chat created", filter.GroupChatCreated(), func(m *models.Message) { m.GroupChatCreated = true }},
		{"migrate to chat", filter.MigrateToChat(), func(m *models.Message) { m.MigrateToChatID = teleiq.Ptr(int64(-1001)) }},
		{"migrate from chat", filter.MigrateFromChat(), func(m *models.Message) { m.MigrateFromChatID = teleiq.Ptr(int64(-1)) }},
		{"pinned message", filter.PinnedMessage(), func(m *models.Message) { m.PinnedMessage = &models.Message{} }},
		{"video chat started", filter.VideoChatStarted(), func(m *models.Message) { m.VideoChatStarted = &models.VideoChatStarted{} }},
		{"video chat ended", filter.VideoChatEnded(), func(m *models.Message) { m.VideoChatEnded = &models.VideoChatEnded{} }},
		{"video chat participants invited", filter.VideoChatParticipantsInvited(), func(m *models.Message) {
			m.VideoChatParticipantsInvited = &models.VideoChatParticipantsInvited{}
		}},
		{"service", filter.Service(), func(m *models.Message) { m.LeftChatMember = &models.User{ID: 8} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			with := newMessage(tt.set)
			updates := []struct {
				name string
				u    models.Update
				want bool
			}{
				{"new message", models.Update{Message: with}, true},
				{"message without it", models.Update{Message: newMessage(nil)}, false},
				{"edit", models.Update{EditedMessage: with}, false},
				{"channel post", models.Update{ChannelPost: with}, false},
				{"business message", models.Update{BusinessMessage: with}, false},
				{"button press", models.Update{CallbackQuery: &models.CallbackQuery{Message: with}}, false},
			}
			for _, u := range updates {
				if got := tt.filter.Match(b.NewContext(&u.u)); got != u.want {
					t.Errorf("%s: matched %t, want %t", u.name, got, u.want)
				}
			}
		})
	}
}

// TestMessageFilterEdges checks the cases where a field is not enough: the fields that Telegram
// fills along with others for compatibility, and the values that must not match.
func TestMessageFilterEdges(t *testing.T) {
	b := newTestBot(t)
	gif := func(m *models.Message) { m.Animation, m.Document = &models.Animation{}, &models.Document{} }
	live := func(m *models.Message) { m.LivePhoto, m.Photo = &models.LivePhoto{}, []models.PhotoSize{{}} }
	venue := func(m *models.Message) { m.Venue, m.Location = &models.Venue{}, &models.Location{} }
	caption := func(m *models.Message) {
		m.Photo, m.Caption = []models.PhotoSize{{}}, teleiq.Ptr("hello")
	}
	tests := []struct {
		name   string
		filter bot.Filter
		set    func(m *models.Message)
		want   bool
	}{
		{"a GIF is an animation", filter.Animation(), gif, true},
		{"a GIF is not a document", filter.Document(), gif, false},
		{"a live photo is not a photo", filter.Photo(), live, false},
		{"a live photo is a live photo", filter.LivePhoto(), live, true},
		{"a venue is not a location", filter.Location(), venue, false},
		{"a venue is a venue", filter.Venue(), venue, true},
		{"media: a GIF", filter.Media(), gif, true},
		{"media: a live photo", filter.Media(), live, true},
		{"media: a live photo alone", filter.Media(), func(m *models.Message) { m.LivePhoto = &models.LivePhoto{} }, true},
		{"media: a photo", filter.Media(), func(m *models.Message) { m.Photo = []models.PhotoSize{{}} }, true},
		{"media: a document", filter.Media(), func(m *models.Message) { m.Document = &models.Document{} }, true},
		{"media: an audio file", filter.Media(), func(m *models.Message) { m.Audio = &models.Audio{} }, true},
		{"media: a sticker", filter.Media(), func(m *models.Message) { m.Sticker = &models.Sticker{} }, true},
		{"media: a video", filter.Media(), func(m *models.Message) { m.Video = &models.Video{} }, true},
		{"media: a video note", filter.Media(), func(m *models.Message) { m.VideoNote = &models.VideoNote{} }, true},
		{"media: a venue has no file", filter.Media(), venue, false},
		{"media: a contact has no file", filter.Media(), func(m *models.Message) { m.Contact = &models.Contact{} }, false},
		{"media: paid media cannot be downloaded", filter.Media(), func(m *models.Message) { m.PaidMedia = &models.PaidMediaInfo{} }, false},
		{"an empty caption", filter.Caption(), func(m *models.Message) { m.Caption = teleiq.Ptr("") }, false},
		{"a message from a user", filter.FromBot(), nil, false},
		{"a message without a sender", filter.FromBot(), func(m *models.Message) { m.From = nil }, false},
		{"regex on another text", filter.Regex(regexp.MustCompile(`^h`)), func(m *models.Message) { m.Text = teleiq.Ptr("oh") }, false},
		{"regex on a caption", filter.Regex(regexp.MustCompile(`hello`)), caption, false},
		{"regex that matches an empty text", filter.Regex(regexp.MustCompile(`^$`)), nil, false},
		{"nil regex", filter.Regex(nil), func(m *models.Message) { m.Text = teleiq.Ptr("hi") }, false},
		{"nil message function", filter.Message(nil), nil, false},
		{"message function on a caption", filter.Message(func(m *models.Message) bool { return m.Caption != nil }), caption, true},
		{"service: a text", filter.Service(), func(m *models.Message) { m.Text = teleiq.Ptr("hi") }, false},
		{"service: a photo", filter.Service(), caption, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Match(b.NewContext(&models.Update{Message: newMessage(tt.set)})); got != tt.want {
				t.Errorf("matched %t, want %t", got, tt.want)
			}
		})
	}
}
