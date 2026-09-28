package main

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestCommands(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })

	if cmds := srv.Wait(t, "setMyCommands", 1)[0].Params["commands"].([]any); len(cmds) != 3 {
		t.Errorf("setMyCommands registered %d commands, want 3", len(cmds))
	}
	messages := []struct{ text, want string }{
		{"/start", "Hi! Send /help to see what I can do."},
		{"/echo hello   there", "hello   there"},
		{"/echo", "Send /echo followed by some text."},
		{"/unknown", "Unknown command. Send /help."},
		{"just text", ""}, // no reply
		{"/help@test_bot", "/start says hello\n/help lists the commands\n/echo <text> repeats the text"},
	}
	var want []string
	for _, m := range messages {
		srv.Push(teleiqtest.MessageUpdate(7, m.text))
		if m.want != "" {
			want = append(want, m.want)
		}
	}
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
