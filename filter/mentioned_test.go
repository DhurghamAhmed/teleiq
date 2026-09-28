package filter_test

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestMentioned(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	b := bot.New(client)
	before := teleiqtest.GroupMessageUpdate(-5, 7, "@test_bot hi")
	before.Message.Entities = []models.MessageEntity{{Type: "mention", Offset: 0, Length: 9}}
	if filter.Mentioned().Match(b.NewContext(&before)) {
		t.Error("Mentioned matched before Run, when the bot does not know its username")
	}

	// A mention and its entity, which Telegram measures in UTF-16 code units.
	mention := func(text string, offset, length int) models.Update {
		u := teleiqtest.GroupMessageUpdate(-5, 7, text)
		u.Message.Entities = []models.MessageEntity{{Type: "mention", Offset: offset, Length: length}}
		return u
	}
	textMention := func(id int64) models.Update {
		u := teleiqtest.GroupMessageUpdate(-5, 7, "hi bot")
		u.Message.Entities = []models.MessageEntity{{Type: "text_mention", Offset: 3, Length: 3, User: &models.User{ID: id}}}
		return u
	}
	replyTo := func(id int64) models.Update {
		u := teleiqtest.GroupMessageUpdate(-5, 7, "ok")
		u.Message.ReplyToMessage = &models.Message{From: &models.User{ID: id, IsBot: true}}
		return u
	}
	caption := models.Update{Message: &models.Message{Chat: models.Chat{ID: -5, Type: "group"}, Photo: []models.PhotoSize{{}},
		Caption: teleiq.Ptr("look @test_bot"), CaptionEntities: []models.MessageEntity{{Type: "mention", Offset: 5, Length: 9}}}}
	edited := mention("@test_bot", 0, 9)
	edited.EditedMessage, edited.Message = edited.Message, nil
	tests := []struct {
		name string
		u    models.Update
		want bool
	}{
		{"mention", mention("hi @test_bot", 3, 9), true},
		{"mention in another case", mention("hi @Test_Bot", 3, 9), true},
		{"mention after an emoji", mention("😀 @test_bot", 3, 9), true},
		{"mention in a caption", caption, true},
		{"text mention", textMention(123456), true},
		{"reply to the bot", replyTo(123456), true},
		{"mention of another bot", mention("hi @test_bot2", 3, 10), false},
		{"text mention of another user", textMention(8), false},
		{"reply to another bot", replyTo(8), false},
		{"username without an entity", teleiqtest.GroupMessageUpdate(-5, 7, "@test_bot hi"), false},
		{"entity past the end of the text", mention("@test_bot", 0, 20), false},
		{"edit", edited, false},
	}
	matched := map[int64]bool{} // by update_id, written by the handler before Run returns
	b.Handle(bot.FilterFunc(func(*bot.Context) bool { return true }), func(_ context.Context, c *bot.Context) error {
		matched[c.Update().UpdateID] = filter.Mentioned().Match(c)
		return nil
	})
	stop := teleiqtest.Start(t, b.Run)
	ids := make([]int64, len(tests))
	for i, tt := range tests {
		ids[i] = srv.Push(tt.u)
		srv.WaitHandled(t, ids[i])
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if len(matched) != len(tests) {
		t.Fatalf("the handler ran for %d updates, want %d", len(matched), len(tests))
	}
	for i, tt := range tests {
		if matched[ids[i]] != tt.want {
			t.Errorf("%s: matched %t, want %t", tt.name, matched[ids[i]], tt.want)
		}
	}
}
