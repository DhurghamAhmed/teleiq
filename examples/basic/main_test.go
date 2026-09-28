package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		chat      string
		fail      map[string]*teleiq.Error // the methods that fail, and how
		wantChat  any                      // the chat_id of the message sent, if one is
		wantOut   string
		is        error
		wantInErr string
	}{
		{
			name: "token only", token: teleiqtest.Token,
			wantOut: "Authorized as Test Bot (id 123456)\nSet CHAT_ID to send a message as well.\n",
		},
		{
			name: "numeric chat", token: teleiqtest.Token, chat: "42", wantChat: 42.0,
			wantOut: "Authorized as Test Bot (id 123456)\nSent message 1 to chat 42\n",
		},
		{
			name: "channel username", token: teleiqtest.Token, chat: "@channel", wantChat: "@channel",
			wantOut: "Authorized as Test Bot (id 123456)\nSent message 1 to chat -1001000000000\n",
		},
		{
			name: "invalid chat", token: teleiqtest.Token, chat: "channel",
			wantOut: "Authorized as Test Bot (id 123456)\n", wantInErr: "CHAT_ID must be",
		},
		{
			name: "user has not started the bot", token: teleiqtest.Token, chat: "42", wantChat: 42.0,
			fail:    map[string]*teleiq.Error{"sendMessage": {ErrorCode: 403, Description: "Forbidden: bot can't initiate conversation with a user"}},
			wantOut: "Authorized as Test Bot (id 123456)\n", is: teleiq.ErrForbidden, wantInErr: "can't initiate conversation",
		},
		{
			name: "rejected token", token: teleiqtest.Token,
			fail: map[string]*teleiq.Error{"getMe": {ErrorCode: 401, Description: "Unauthorized"}},
			is:   teleiq.ErrUnauthorized, wantInErr: "rejected BOT_TOKEN",
		},
		{name: "missing token", wantInErr: "set BOT_TOKEN"},
		{name: "malformed token", token: "not a token", is: teleiq.ErrInvalidToken, wantInErr: "invalid bot token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := teleiqtest.NewServer()
			defer srv.Close()
			for method, err := range tt.fail {
				srv.Fail(method, err)
			}
			var out bytes.Buffer

			err := run(context.Background(), tt.token, tt.chat, log.New(&out, "", 0), teleiq.WithBaseURL(srv.URL))

			if tt.wantInErr == "" && err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if tt.wantInErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantInErr)) {
				t.Fatalf("run() error = %v, want one mentioning %q", err, tt.wantInErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.is)
			}
			sent := srv.Requests("sendMessage")
			switch {
			case tt.wantChat == nil && len(sent) > 0:
				t.Errorf("sent %v, want no message", sent)
			case tt.wantChat != nil && (len(sent) != 1 || sent[0].Params["chat_id"] != tt.wantChat || sent[0].Params["text"] != "Hello from Test Bot!"):
				t.Errorf("sent %v, want one greeting to chat %v", sent, tt.wantChat)
			}
			if out.String() != tt.wantOut {
				t.Errorf("output = %q, want %q", out.String(), tt.wantOut)
			}
		})
	}
}
