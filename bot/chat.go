package bot

import (
	"context"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// MemberOption changes how Ban and Restrict treat a member.
type MemberOption func(*memberConfig)

type memberConfig struct {
	until *int64
}

// Until makes a ban or a restriction end at t.
func Until(t time.Time) MemberOption {
	return func(m *memberConfig) { m.until = teleiq.Ptr(t.Unix()) }
}

func memberOptions(opts []MemberOption) memberConfig {
	var m memberConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&m)
		}
	}
	return m
}

// chatID returns the chat of the update; method names the caller in errors.
func (c *Context) chatID(method string) (models.ChatID, error) {
	d, err := c.destination(method)
	return d.chat, err
}

// Ban bans the user from the chat of the update.
func (c *Context) Ban(ctx context.Context, userID int64, opts ...MemberOption) error {
	chat, err := c.chatID("Ban")
	if err != nil {
		return err
	}
	m := memberOptions(opts)
	return c.bot.client.BanChatMember(ctx, teleiq.BanChatMemberParams{ChatID: chat, UserID: userID, UntilDate: m.until})
}

// Unban lifts the ban of the user from the chat of the update.
func (c *Context) Unban(ctx context.Context, userID int64) error {
	chat, err := c.chatID("Unban")
	if err != nil {
		return err
	}
	return c.bot.client.UnbanChatMember(ctx, teleiq.UnbanChatMemberParams{ChatID: chat, UserID: userID, OnlyIfBanned: teleiq.Ptr(true)})
}

// Restrict sets what the user may do in the chat of the update.
func (c *Context) Restrict(ctx context.Context, userID int64, perms models.ChatPermissions, opts ...MemberOption) error {
	chat, err := c.chatID("Restrict")
	if err != nil {
		return err
	}
	m := memberOptions(opts)
	return c.bot.client.RestrictChatMember(ctx, teleiq.RestrictChatMemberParams{ChatID: chat, UserID: userID, Permissions: perms, UntilDate: m.until})
}

// Member returns the membership of the user in the chat of the update.
func (c *Context) Member(ctx context.Context, userID int64) (models.ChatMember, error) {
	chat, err := c.chatID("Member")
	if err != nil {
		return nil, err
	}
	return c.bot.client.GetChatMember(ctx, teleiq.GetChatMemberParams{ChatID: chat, UserID: userID})
}

// Leave makes the bot leave the chat of the update.
func (c *Context) Leave(ctx context.Context) error {
	chat, err := c.chatID("Leave")
	if err != nil {
		return err
	}
	return c.bot.client.LeaveChat(ctx, teleiq.LeaveChatParams{ChatID: chat})
}
