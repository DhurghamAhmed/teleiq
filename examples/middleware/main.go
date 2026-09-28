// Command middleware shows Recovery, Logging and a middleware that keeps to private chats.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
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
	b := bot.New(client) // b is the bot
	// Recovery comes first, so it also catches panics in the middleware after it.
	b.Use(bot.Recovery(), bot.Logging(), privateOnly)
	b.OnCommand("ping", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; c.Send replies in its chat.
		return c.Send(ctx, "pong")
	})
	b.OnCommand("panic", func(context.Context, *bot.Context) error {
		panic("something went wrong") // Recovery catches this and the bot keeps running
	})
	return b.Run(ctx) // receives updates until ctx ends
}

// private matches updates from private chats.
var private = filter.Private()

// privateOnly answers groups and channels itself; next is the rest of the chain.
func privateOnly(next bot.Handler) bot.Handler {
	return func(ctx context.Context, c *bot.Context) error {
		if c.Chat() != nil && !private.Match(c) {
			return c.Send(ctx, "Please talk to me in a private chat.") // next never runs
		}
		return next(ctx, c)
	}
}
