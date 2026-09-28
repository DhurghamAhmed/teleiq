package bot_test

import (
	"context"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

var (
	groupMessage = &models.Update{UpdateID: 1, Message: &models.Message{MessageID: 5, Date: 1,
		Chat: models.Chat{ID: -100, Type: "supergroup"}, From: &models.User{ID: 7, FirstName: "Ann"}, Text: teleiq.Ptr("/ban")}}
	businessMessage = &models.Update{UpdateID: 1, BusinessMessage: business}
	joinRequest     = func() *models.Update { u := teleiqtest.JoinRequestUpdate(-1001234567890, 8); return &u }()
	until           = time.Unix(2000000000, 0)
)

func TestMessageShortcuts(t *testing.T) {
	deleteOf := func(ctx context.Context, c *bot.Context) error { return c.Delete(ctx) }
	tests := []struct {
		name, method string
		update       *models.Update
		call         func(ctx context.Context, c *bot.Context) error
		fail         *teleiq.Error
		wantErr      bool
		want         string // the parameters of method, or "" for no request
	}{
		{name: "delete a message", method: "deleteMessage", update: textMessage, call: deleteOf, want: "map[chat_id:7 message_id:1]"},
		{name: "delete the message of a button", method: "deleteMessage", update: press(accessible, ""), call: deleteOf, want: "map[chat_id:7 message_id:9]"},
		{name: "delete an inaccessible message", method: "deleteMessage", update: press(inaccessible, ""), call: deleteOf, want: "map[chat_id:7 message_id:9]"},
		{name: "delete a business message", method: "deleteBusinessMessages", update: businessMessage, call: deleteOf, want: "map[business_connection_id:biz message_ids:[9]]"},
		{name: "delete an inline message", method: "deleteMessage", update: press(nil, "inl"), call: deleteOf, wantErr: true},
		{name: "delete without a message", method: "deleteMessage", update: inlineQuery, call: deleteOf, wantErr: true},
		{
			name: "delete a message already deleted", method: "deleteMessage", update: textMessage, call: deleteOf,
			fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: message to delete not found"}, want: "map[chat_id:7 message_id:1]",
		},
		{
			name: "delete a message too old", method: "deleteMessage", update: textMessage, call: deleteOf,
			fail: &teleiq.Error{ErrorCode: 400, Description: "Bad Request: message can't be deleted"}, wantErr: true, want: "map[chat_id:7 message_id:1]",
		},
		{
			name: "forward", method: "forwardMessage", update: textMessage, want: "map[chat_id:42 from_chat_id:7 message_id:1]",
			call: func(ctx context.Context, c *bot.Context) error { _, err := c.Forward(ctx, models.ID(42)); return err },
		},
		{
			name: "forward to a username", method: "forwardMessage", update: press(inaccessible, ""), want: "map[chat_id:@news from_chat_id:7 message_id:9]",
			call: func(ctx context.Context, c *bot.Context) error {
				_, err := c.Forward(ctx, models.Username("@news"))
				return err
			},
		},
		{
			name: "forward without a message", method: "forwardMessage", update: inlineQuery, wantErr: true,
			call: func(ctx context.Context, c *bot.Context) error { _, err := c.Forward(ctx, models.ID(42)); return err },
		},
		{
			name: "copy", method: "copyMessage", update: groupMessage, want: "map[chat_id:42 from_chat_id:-100 message_id:5]",
			call: func(ctx context.Context, c *bot.Context) error { _, err := c.Copy(ctx, models.ID(42)); return err },
		},
		{
			name: "copy without a message", method: "copyMessage", update: joinRequest, wantErr: true,
			call: func(ctx context.Context, c *bot.Context) error { _, err := c.Copy(ctx, models.ID(42)); return err },
		},
		{
			name: "pin", method: "pinChatMessage", update: groupMessage, want: "map[chat_id:-100 message_id:5]",
			call: func(ctx context.Context, c *bot.Context) error { return c.Pin(ctx) },
		},
		{
			name: "pin a business message", method: "pinChatMessage", update: businessMessage, want: "map[business_connection_id:biz chat_id:7 message_id:9]",
			call: func(ctx context.Context, c *bot.Context) error { return c.Pin(ctx) },
		},
		{
			name: "unpin", method: "unpinChatMessage", update: press(accessible, ""), want: "map[chat_id:7 message_id:9]",
			call: func(ctx context.Context, c *bot.Context) error { return c.Unpin(ctx) },
		},
		{
			name: "react", method: "setMessageReaction", update: groupMessage, want: "map[chat_id:-100 message_id:5 reaction:[map[emoji:👍 type:emoji]]]",
			call: func(ctx context.Context, c *bot.Context) error { return c.React(ctx, "👍") },
		},
		{
			name: "remove a reaction", method: "setMessageReaction", update: groupMessage, want: "map[chat_id:-100 message_id:5 reaction:[]]",
			call: func(ctx context.Context, c *bot.Context) error { return c.React(ctx, "") },
		},
		{
			name: "react without a message", method: "setMessageReaction", update: inlineQuery, wantErr: true,
			call: func(ctx context.Context, c *bot.Context) error { return c.React(ctx, "👍") },
		},
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

func TestMemberShortcuts(t *testing.T) {
	ban := func(opts ...bot.MemberOption) func(ctx context.Context, c *bot.Context) error {
		return func(ctx context.Context, c *bot.Context) error { return c.Ban(ctx, 8, opts...) }
	}
	restrict := func(perms models.ChatPermissions, opts ...bot.MemberOption) func(ctx context.Context, c *bot.Context) error {
		return func(ctx context.Context, c *bot.Context) error { return c.Restrict(ctx, 8, perms, opts...) }
	}
	approve := func(ctx context.Context, c *bot.Context) error { return c.Approve(ctx) }
	decline := func(ctx context.Context, c *bot.Context) error { return c.Decline(ctx) }
	tests := []struct {
		name, method string
		update       *models.Update
		call         func(ctx context.Context, c *bot.Context) error
		wantErr      bool
		want         string
	}{
		{name: "ban", method: "banChatMember", update: groupMessage, call: ban(), want: "map[chat_id:-100 user_id:8]"},
		{name: "ban until", method: "banChatMember", update: groupMessage, call: ban(bot.Until(until), nil), want: "map[chat_id:-100 until_date:2e+09 user_id:8]"},
		{name: "ban from the chat of a button", method: "banChatMember", update: press(inaccessible, ""), call: ban(), want: "map[chat_id:7 user_id:8]"},
		{name: "ban without a chat", method: "banChatMember", update: inlineQuery, call: ban(), wantErr: true},
		{
			name: "unban only if banned", method: "unbanChatMember", update: groupMessage, want: "map[chat_id:-100 only_if_banned:true user_id:8]",
			call: func(ctx context.Context, c *bot.Context) error { return c.Unban(ctx, 8) },
		},
		{name: "mute", method: "restrictChatMember", update: groupMessage, call: restrict(models.ChatPermissions{}), want: "map[chat_id:-100 permissions:map[] user_id:8]"},
		{
			name: "restrict until", method: "restrictChatMember", update: groupMessage, call: restrict(models.ChatPermissions{CanSendMessages: teleiq.Ptr(true)}, bot.Until(until)),
			want: "map[chat_id:-100 permissions:map[can_send_messages:true] until_date:2e+09 user_id:8]",
		},
		{name: "restrict without a chat", method: "restrictChatMember", update: inlineQuery, call: restrict(models.ChatPermissions{}), wantErr: true},
		{
			name: "member", method: "getChatMember", update: groupMessage, want: "map[chat_id:-100 user_id:8]",
			call: func(ctx context.Context, c *bot.Context) error {
				m, err := c.Member(ctx, 8)
				if _, ok := m.(*models.ChatMemberMember); err == nil && !ok {
					t.Errorf("Member() = %T, want *models.ChatMemberMember", m)
				}
				return err
			},
		},
		{
			name: "member without a chat", method: "getChatMember", update: inlineQuery, wantErr: true,
			call: func(ctx context.Context, c *bot.Context) error { _, err := c.Member(ctx, 8); return err },
		},
		{
			name: "leave", method: "leaveChat", update: groupMessage, want: "map[chat_id:-100]",
			call: func(ctx context.Context, c *bot.Context) error { return c.Leave(ctx) },
		},
		{name: "approve", method: "approveChatJoinRequest", update: joinRequest, call: approve, want: "map[chat_id:-1.00123456789e+12 user_id:8]"},
		{name: "approve a message", method: "approveChatJoinRequest", update: groupMessage, call: approve, wantErr: true},
		{name: "decline", method: "declineChatJoinRequest", update: joinRequest, call: decline, want: "map[chat_id:-1.00123456789e+12 user_id:8]"},
		{name: "decline a message", method: "declineChatJoinRequest", update: groupMessage, call: decline, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, b := newBot(t)
			err := tt.call(t.Context(), b.NewContext(tt.update))
			checkCall(t, srv, tt.method, err, nil, tt.wantErr, tt.want)
		})
	}
}
