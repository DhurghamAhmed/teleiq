package bot_test

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestEditCaptionAndKeyboard(t *testing.T) {
	keyboard := models.NewInlineKeyboard(models.NewInlineRow(models.NewCallbackButton("A", "a")))
	caption := func(opts ...bot.SendOption) func(ctx context.Context, c *bot.Context) error {
		return func(ctx context.Context, c *bot.Context) error { return c.EditCaption(ctx, "new", opts...) }
	}
	buttons := func(kb *models.InlineKeyboardMarkup) func(ctx context.Context, c *bot.Context) error {
		return func(ctx context.Context, c *bot.Context) error { return c.EditKeyboard(ctx, kb) }
	}
	notModifiedErr := &teleiq.Error{ErrorCode: 400, Description: notModified}
	tests := []struct {
		name, method string
		update       *models.Update
		call         func(ctx context.Context, c *bot.Context) error
		fail         *teleiq.Error
		wantErr      bool
		want         string
	}{
		{name: "caption", method: "editMessageCaption", update: press(accessible, ""), call: caption(), want: "map[caption:new chat_id:7 message_id:9]"},
		{name: "caption of an inline message", method: "editMessageCaption", update: press(nil, "inl"), call: caption(), want: "map[caption:new inline_message_id:inl]"},
		{name: "caption of a business message", method: "editMessageCaption", update: press(business, ""), call: caption(), want: "map[business_connection_id:biz caption:new chat_id:7 message_id:9]"},
		{name: "caption of an inaccessible message", method: "editMessageCaption", update: press(inaccessible, ""), call: caption(), want: "map[caption:new chat_id:7 message_id:9]"},
		{name: "caption in HTML", method: "editMessageCaption", update: press(accessible, ""), call: caption(bot.HTML()), want: "map[caption:new chat_id:7 message_id:9 parse_mode:HTML]"},
		{
			name: "caption with entities and a keyboard", method: "editMessageCaption", update: press(accessible, ""),
			call: caption(bot.Keyboard(keyboard), func(p *teleiq.SendMessageParams) { p.Entities = []models.MessageEntity{{Type: "bold", Length: 3}} }),
			want: "map[caption:new caption_entities:[map[length:3 offset:0 type:bold]] chat_id:7 message_id:9 reply_markup:map[inline_keyboard:[[map[callback_data:a text:A]]]]]",
		},
		{name: "caption with a reply keyboard", method: "editMessageCaption", update: press(accessible, ""), call: caption(bot.Keyboard(&models.ReplyKeyboardMarkup{})), wantErr: true},
		{name: "caption without a callback query", method: "editMessageCaption", update: textMessage, call: caption(), wantErr: true},
		{name: "caption of a callback query without a message", method: "editMessageCaption", update: press(nil, ""), call: caption(), wantErr: true},
		{name: "caption not modified", method: "editMessageCaption", update: press(accessible, ""), call: caption(), fail: notModifiedErr, want: "map[caption:new chat_id:7 message_id:9]"},
		{
			name: "caption of a message not found", method: "editMessageCaption", update: press(accessible, ""), call: caption(),
			fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: message to edit not found"}, wantErr: true, want: "map[caption:new chat_id:7 message_id:9]",
		},
		{
			name: "keyboard", method: "editMessageReplyMarkup", update: press(accessible, ""), call: buttons(keyboard),
			want: "map[chat_id:7 message_id:9 reply_markup:map[inline_keyboard:[[map[callback_data:a text:A]]]]]",
		},
		{name: "no keyboard", method: "editMessageReplyMarkup", update: press(accessible, ""), call: buttons(nil), want: "map[chat_id:7 message_id:9]"},
		{name: "keyboard of an inline message", method: "editMessageReplyMarkup", update: press(nil, "inl"), call: buttons(nil), want: "map[inline_message_id:inl]"},
		{name: "keyboard of a business message", method: "editMessageReplyMarkup", update: press(business, ""), call: buttons(nil), want: "map[business_connection_id:biz chat_id:7 message_id:9]"},
		{name: "keyboard without a callback query", method: "editMessageReplyMarkup", update: textMessage, call: buttons(nil), wantErr: true},
		{name: "keyboard not modified", method: "editMessageReplyMarkup", update: press(accessible, ""), call: buttons(nil), fail: notModifiedErr, want: "map[chat_id:7 message_id:9]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			if tt.fail != nil {
				srv.Fail(tt.method, tt.fail)
			}
			err := tt.call(t.Context(), b.NewContext(tt.update))
			checkCall(t, srv, tt.method, err, tt.fail, tt.wantErr, tt.want)
		})
	}
}
