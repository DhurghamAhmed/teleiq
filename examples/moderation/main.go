// Command moderation bans with /ban, mutes with /mute and keeps a group tidy.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
)

func main() {
	// ctx ends on Ctrl+C or SIGTERM, which stops the bot.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv("BOT_TOKEN"))
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, opts ...teleiq.Option) error {
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	b := bot.New(client)  // b is the bot
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	// c is the update being handled, here a request to join.
	b.OnJoinRequest(func(ctx context.Context, c *bot.Context) error { return c.Approve(ctx) })

	// groups holds the handlers that run only in groups and supergroups.
	groups := b.Group(filter.Group())
	groups.Handle(filter.Or(filter.NewChatMembers(), filter.LeftChatMember()), func(ctx context.Context, c *bot.Context) error {
		return c.Delete(ctx)
	})
	groups.OnCommand("ban", punish(func(ctx context.Context, c *bot.Context, user int64) error {
		return c.Ban(ctx, user)
	}))
	groups.OnCommand("mute", punish(func(ctx context.Context, c *bot.Context, user int64) error {
		// No permissions at all, until an hour from now.
		return c.Restrict(ctx, user, models.ChatPermissions{}, bot.Until(time.Now().Add(time.Hour)))
	}))
	return b.Run(ctx) // receives updates until ctx ends
}

// punish returns a handler that applies act to the member the command answers.
func punish(act func(ctx context.Context, c *bot.Context, user int64) error) bot.Handler {
	return func(ctx context.Context, c *bot.Context) error {
		replied := c.Message().ReplyToMessage // the message that the command answers
		if replied == nil || replied.From == nil {
			return c.Send(ctx, "Answer the message of the member with /"+c.Command()+".")
		}
		err := act(ctx, c, replied.From.ID)
		switch {
		case errors.Is(err, teleiq.ErrMemberIsAdmin):
			return c.Send(ctx, "I cannot do that to an administrator.")
		case errors.Is(err, teleiq.ErrNotEnoughRights):
			return c.Send(ctx, "Make me an administrator who may ban members first.")
		case err != nil:
			return err
		}
		return c.Send(ctx, "Done.")
	}
}
