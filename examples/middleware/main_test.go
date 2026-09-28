package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestMiddleware(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(old)

	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })

	srv.Push(teleiqtest.MessageUpdate(7, "/panic"))
	srv.Push(teleiqtest.MessageUpdate(7, "/ping"))
	srv.Push(teleiqtest.GroupMessageUpdate(-100, 7, "/ping"))

	// The two chats are handled in parallel, so their replies come in any order.
	replies := map[any]any{}
	for _, r := range srv.Wait(t, "sendMessage", 2) {
		replies[r.Params["chat_id"]] = r.Params["text"]
	}
	if replies[7.0] != "pong" || replies[-100.0] != "Please talk to me in a private chat." {
		t.Errorf("replies = %v, want pong after the panic and the refusal in the group", replies)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`msg="bot: handler panicked"`, `msg="bot: update handled"`, "kind=message"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs lack %s:\n%s", want, logs.String())
		}
	}
}
