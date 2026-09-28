// Command commands answers /start, /help and /echo, listed in the command menu.
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

// commands is the menu that Telegram shows when the user types /.
var commands = []models.BotCommand{
	{Command: "start", Description: "Say hello"},
	{Command: "help", Description: "List the commands"},
	{Command: "echo", Description: "Repeat the text after the command"},
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, opts ...teleiq.Option) error {
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	// b is the bot; WithCommands sets its command menu when it starts.
	b := bot.New(client, bot.WithCommands(commands...))
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	// ReplyWith is a handler that always sends the same text.
	b.OnCommand("start", bot.ReplyWith("Hi! Send /help to see what I can do."))
	b.OnCommand("help", bot.ReplyWith("/start says hello\n/help lists the commands\n/echo <text> repeats the text"))
	b.OnCommand("echo", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; c.Args is the text after the command.
		if c.Args() == "" {
			return c.Send(ctx, "Send /echo followed by some text.")
		}
		return c.Send(ctx, c.Args())
	})
	// Registered last, so it only gets unknown commands.
	b.Handle(filter.AnyCommand(), bot.ReplyWith("Unknown command. Send /help."))
	return b.Run(ctx) // receives updates until ctx ends
}
