package models

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestChatIDMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		id      ChatID
		want    string
		wantErr bool
	}{
		{name: "user", id: ID(123456789), want: `123456789`},
		{name: "supergroup", id: ID(-1001234567890), want: `-1001234567890`},
		{name: "beyond float precision", id: ID(-9007199254740993), want: `-9007199254740993`},
		{name: "largest id", id: ID(math.MaxInt64), want: `9223372036854775807`},
		{name: "username", id: Username("@channel"), want: `"@channel"`},
		{name: "username escaped", id: Username(`@a"b\c`), want: `"@a\"b\\c"`},
		{name: "zero value", id: ChatID{}, wantErr: true},
		{name: "zero id", id: ID(0), wantErr: true},
		{name: "empty username", id: Username(""), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range []any{tt.id, &tt.id} {
				got, err := json.Marshal(v)
				if tt.wantErr {
					if err == nil || !strings.Contains(err.Error(), "ChatID is not set") {
						t.Fatalf("Marshal(%T) = %s, %v; want a ChatID is not set error", v, got, err)
					}
					continue
				}
				if err != nil || string(got) != tt.want {
					t.Fatalf("Marshal(%T) = %s, %v; want %s", v, got, err, tt.want)
				}
			}
		})
	}
}

func TestChatIDInParams(t *testing.T) {
	type params struct {
		ChatID ChatID `json:"chat_id"`
		From   ChatID `json:"from_chat_id,omitzero"`
	}
	tests := []struct {
		name    string
		in      params
		want    string
		wantErr bool
	}{
		{name: "optional omitted", in: params{ChatID: ID(1)}, want: `{"chat_id":1}`},
		{name: "optional set", in: params{ChatID: Username("@a"), From: ID(-2)}, want: `{"chat_id":"@a","from_chat_id":-2}`},
		{name: "required missing", in: params{From: ID(3)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Marshal() = %s, want an error", got)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Fatalf("Marshal() = %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestChatIDAccessors(t *testing.T) {
	tests := []struct {
		chat     ChatID
		id       int64
		username string
	}{
		{ID(-1001234567890), -1001234567890, ""},
		{Username("@channel"), 0, "@channel"},
		{ChatID{}, 0, ""},
	}
	for _, tt := range tests {
		if id, username := tt.chat.ID(), tt.chat.Username(); id != tt.id || username != tt.username {
			t.Errorf("%+v: ID() = %d, Username() = %q; want %d, %q", tt.chat, id, username, tt.id, tt.username)
		}
	}
}
