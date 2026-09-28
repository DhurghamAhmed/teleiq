package bot_test

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestSend(t *testing.T) {
	keyboard := models.NewInlineKeyboard(models.NewInlineRow(models.NewURLButton("Site", "https://example.com")))
	tests := []struct {
		name    string
		update  *models.Update
		opts    []bot.SendOption
		fail    *teleiq.Error
		wantErr bool
		want    string // the parameters of sendMessage, or "" for no request
	}{
		{name: "message", update: textMessage, want: "map[chat_id:7 text:hi]"},
		{name: "button press", update: press(accessible, ""), want: "map[chat_id:7 text:hi]"},
		{name: "HTML", update: textMessage, opts: []bot.SendOption{bot.HTML()}, want: "map[chat_id:7 parse_mode:HTML text:hi]"},
		{name: "Markdown", update: textMessage, opts: []bot.SendOption{bot.Markdown()}, want: "map[chat_id:7 parse_mode:MarkdownV2 text:hi]"},
		{
			name: "inline keyboard", update: textMessage, opts: []bot.SendOption{bot.Keyboard(keyboard)},
			want: "map[chat_id:7 reply_markup:map[inline_keyboard:[[map[text:Site url:https://example.com]]]] text:hi]",
		},
		{
			name: "keyboard removal", update: textMessage, opts: []bot.SendOption{bot.Keyboard(&models.ReplyKeyboardRemove{})},
			want: "map[chat_id:7 reply_markup:map[remove_keyboard:true] text:hi]",
		},
		{
			name: "the last parse mode wins", update: textMessage, opts: []bot.SendOption{bot.HTML(), nil, bot.Markdown()},
			want: "map[chat_id:7 parse_mode:MarkdownV2 text:hi]",
		},
		{name: "no chat", update: &models.Update{InlineQuery: &models.InlineQuery{ID: "i"}}, wantErr: true},
		{name: "empty update", update: nil, wantErr: true},
		{
			name: "failure", update: textMessage, fail: &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"},
			wantErr: true, want: "map[chat_id:7 text:hi]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail("sendMessage", tt.fail)
			}
			err := b.NewContext(tt.update).Send(context.Background(), "hi", tt.opts...)
			checkCall(t, srv, "sendMessage", err, tt.fail, tt.wantErr, tt.want)
		})
		t.Run(tt.name+" with ReplyWith", func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail("sendMessage", tt.fail)
			}
			err := bot.ReplyWith("hi", tt.opts...)(context.Background(), b.NewContext(tt.update))
			checkCall(t, srv, "sendMessage", err, tt.fail, tt.wantErr, tt.want)
		})
	}
}

func TestText(t *testing.T) {
	chat := models.Chat{ID: 7, Type: "private"}
	text := teleiq.Ptr("hello")
	tests := []struct {
		name   string
		update *models.Update
		want   string
	}{
		{"new message", &models.Update{Message: &models.Message{Chat: chat, Text: text}}, "hello"},
		{"command", &models.Update{Message: &models.Message{Chat: chat, Text: teleiq.Ptr("/start now")}}, "/start now"},
		{"caption", &models.Update{Message: &models.Message{Chat: chat, Caption: text, Photo: []models.PhotoSize{{FileID: "p"}}}}, ""},
		{"edited message", &models.Update{EditedMessage: &models.Message{Chat: chat, Text: text}}, ""},
		{"channel post", &models.Update{ChannelPost: &models.Message{Chat: models.Chat{ID: -5, Type: "channel"}, Text: text}}, ""},
		{"business message", &models.Update{BusinessMessage: &models.Message{Chat: chat, Text: text}}, ""},
		{"button press", &models.Update{CallbackQuery: &models.CallbackQuery{ID: "q", Data: text, Message: &models.Message{Date: 1, Chat: chat, Text: text}}}, ""},
		{"inline query", &models.Update{InlineQuery: &models.InlineQuery{ID: "i", Query: "hello"}}, ""},
		{"empty update", nil, ""},
	}
	_, b := newBot(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.NewContext(tt.update).Text(); got != tt.want {
				t.Errorf("Text = %q, want %q", got, tt.want)
			}
		})
	}
}
