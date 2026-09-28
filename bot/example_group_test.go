package bot_test

import (
	"context"
	"fmt"
	"log"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// Group puts handlers under one filter and middleware, here the commands of the administrator
// with ID 1. An update that the filter matches but no handler of the group does, such as /start
// from the administrator, goes on to the handlers added after the group.
func ExampleBot_Group() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	b := bot.New(client)

	admins := b.Group(filter.UserID(1))
	admins.Use(func(next bot.Handler) bot.Handler {
		return func(ctx context.Context, c *bot.Context) error {
			fmt.Println("admin command:", c.Command())
			return next(ctx, c)
		}
	})
	admins.OnCommand("ban", bot.ReplyWith("Banned."))
	b.OnCommand("ban", bot.ReplyWith("Only administrators can ban."))
	b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
		defer stop() // the last update of this example
		return c.Send(ctx, "Hello!")
	})

	// In one group, so that the updates are handled in order.
	srv.Push(teleiqtest.GroupMessageUpdate(-100, 1, "/ban"))
	srv.Push(teleiqtest.GroupMessageUpdate(-100, 2, "/ban"))
	srv.Push(teleiqtest.GroupMessageUpdate(-100, 1, "/start"))
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// admin command: ban
	// Banned.
	// Only administrators can ban.
	// Hello!
}
