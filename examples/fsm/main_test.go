package main

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestRegistration(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })

	messages := []struct{ text, want string }{
		{"/register", "What is your name?"},
		{"/help", ""}, // a command is not an answer
		{"Ann", "How old are you?"},
		{"thirty", "Please send your age as a number."},
		{"30", "Registered Ann, 30 years old."},
		{"31", ""}, // the conversation is over
		{"/register", "What is your name?"},
		{"/cancel", "Cancelled."},
		{"Bob", ""},
	}
	var want []string
	for _, m := range messages {
		srv.Push(teleiqtest.MessageUpdate(7, m.text))
		if m.want != "" {
			want = append(want, m.want)
		}
	}
	srv.Push(teleiqtest.MessageUpdate(8, "Carol")) // another user, in no conversation
	srv.Push(teleiqtest.MessageUpdate(7, "/cancel"))
	want = append(want, "Cancelled.")

	sent := srv.Wait(t, "sendMessage", len(want))
	if len(sent) != len(want) {
		t.Fatalf("%d replies, want %d", len(sent), len(want))
	}
	for i, w := range want {
		if got := sent[i].Params["text"]; got != w {
			t.Errorf("reply %d = %q, want %q", i, got, w)
		}
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
