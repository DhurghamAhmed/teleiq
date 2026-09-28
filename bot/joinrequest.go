package bot

import (
	"context"
	"errors"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// OnJoinRequest runs h for requests to join a chat that the bot administers.
func (b *Bot) OnJoinRequest(h Handler) {
	b.Handle(onJoinRequest, h)
}

// Approve accepts the request to join a chat of the update.
func (c *Context) Approve(ctx context.Context) error {
	r := c.update.ChatJoinRequest
	if r == nil {
		return errors.New("bot: Approve: the update is not a join request")
	}
	return c.bot.client.ApproveChatJoinRequest(ctx, teleiq.ApproveChatJoinRequestParams{ChatID: models.ID(r.Chat.ID), UserID: r.From.ID})
}

// Decline refuses the request to join a chat of the update.
func (c *Context) Decline(ctx context.Context) error {
	r := c.update.ChatJoinRequest
	if r == nil {
		return errors.New("bot: Decline: the update is not a join request")
	}
	return c.bot.client.DeclineChatJoinRequest(ctx, teleiq.DeclineChatJoinRequestParams{ChatID: models.ID(r.Chat.ID), UserID: r.From.ID})
}
