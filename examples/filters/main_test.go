package main

import (
	"context"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestFilters(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })
	// media returns a private message without text, with what set adds.
	media := func(set func(m *models.Message)) models.Update {
		u := teleiqtest.MessageUpdate(7, "")
		u.Message.Text = nil
		set(u.Message)
		return u
	}
	// reply returns a message in chat that replies to another message of the chat.
	reply := func(chat int64) models.Update {
		u := teleiqtest.GroupMessageUpdate(chat, 7, "I agree")
		u.Message.ReplyToMessage = &models.Message{MessageID: 1, Chat: u.Message.Chat}
		return u
	}

	cases := []struct {
		name string
		u    models.Update
		want string
	}{
		{"command", teleiqtest.MessageUpdate(7, "/help"), "Send me a photo, a voice message, a number or any text."},
		{"photo", media(func(m *models.Message) { m.Photo = []models.PhotoSize{{FileID: "photo"}} }), "Nice photo!"},
		{"voice", media(func(m *models.Message) { m.Voice = &models.Voice{FileID: "voice"} }), "I got your audio."},
		{"audio", media(func(m *models.Message) { m.Audio = &models.Audio{FileID: "audio"} }), "I got your audio."},
		{"exact text", teleiqtest.MessageUpdate(7, "hi"), "Hello!"},
		{"number", teleiqtest.MessageUpdate(7, "2026"), "That is a number."},
		{"reply in a group", reply(-1001234567890), "You replied to someone in a group."},
		{"reply in private", reply(7), "You said: I agree"},
		{"long text", teleiqtest.MessageUpdate(7, strings.Repeat("a", 101)), "That is a long message."},
		{"text", teleiqtest.MessageUpdate(7, "hello"), "You said: hello"},
		{"sticker", media(func(m *models.Message) { m.Sticker = &models.Sticker{FileID: "sticker"} }), "No filter above matches this kind of message."},
	}
	for _, c := range cases {
		srv.WaitHandled(t, srv.Push(c.u))
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	sent := srv.Requests("sendMessage")
	if len(sent) != len(cases) {
		t.Fatalf("%d replies, want %d", len(sent), len(cases))
	}
	for i, c := range cases {
		if got := sent[i].Params["text"]; got != c.want {
			t.Errorf("%s: reply %q, want %q", c.name, got, c.want)
		}
	}
}
