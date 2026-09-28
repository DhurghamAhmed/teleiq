package main

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestHandlers(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })
	send := func(u models.Update) { srv.WaitHandled(t, srv.Push(u)) }
	photo := teleiqtest.MessageUpdate(7, "")
	photo.Message.Text, photo.Message.Photo = nil, []models.PhotoSize{{FileID: "photo"}}

	send(teleiqtest.MessageUpdate(7, "/start"))
	send(teleiqtest.MessageUpdate(7, "/help"))
	send(teleiqtest.MessageUpdate(7, "/fail"))
	send(teleiqtest.MessageUpdate(7, "/vote"))
	send(teleiqtest.CallbackUpdate(7, 1, "vote:yes"))
	send(teleiqtest.CallbackUpdate(7, 1, "maybe"))
	send(teleiqtest.GroupMessageUpdate(-1001234567890, 7, "/rules"))
	send(teleiqtest.MessageUpdate(7, "/rules"))
	send(photo)
	send(teleiqtest.MessageUpdate(7, "hello"))
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"Hello! Send /help.",
		"/vote shows buttons, /rules works in groups, /fail fails.",
		"Sorry, something went wrong.",
		"Do you like Go?",
		"Be kind and stay on topic.",
		"Rules apply only in groups.",
		"Nice photo!",
		"Send /help to see what I can do.",
	}
	sent := srv.Requests("sendMessage")
	if len(sent) != len(want) {
		t.Fatalf("%d replies, want %d", len(sent), len(want))
	}
	for i, w := range want {
		if got := sent[i].Params["text"]; got != w {
			t.Errorf("reply %d = %q, want %q", i, got, w)
		}
	}
	answers := srv.Requests("answerCallbackQuery")
	if len(answers) != 2 || answers[0].Params["text"] != "You voted yes" || answers[1].Params["text"] != "This button does nothing." {
		t.Errorf("answers to the buttons: %v", answers)
	}
	if edits := srv.Requests("editMessageText"); len(edits) != 1 || edits[0].Params["text"] != "Thanks for voting yes." {
		t.Errorf("edits: %v", edits)
	}
}
