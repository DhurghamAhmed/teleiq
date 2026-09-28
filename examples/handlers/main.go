// Command handlers shows each way to add a handler and the order in which they match.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
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

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, opts ...teleiq.Option) error {
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	b := bot.New(client, bot.WithErrorHandler(reportError)) // b is the bot
	b.Use(bot.Recovery())                                   // a panic in a handler does not stop the bot

	// Each update goes to the first handler below that matches it.
	b.OnCommand("start", start)
	b.OnCommand("help", bot.ReplyWith("/vote shows buttons, /rules works in groups, /fail fails."))
	b.OnCommand("fail", func(context.Context, *bot.Context) error {
		return errors.New("the database is down") // goes to reportError
	})
	b.OnCommand("vote", vote)
	b.OnCallbackPrefix("vote:", func(ctx context.Context, c *bot.Context) error {
		choice := strings.TrimPrefix(c.CallbackData(), "vote:")
		return errors.Join(c.Answer(ctx, "You voted "+choice), c.Edit(ctx, "Thanks for voting "+choice+"."))
	})
	b.OnCallback(func(ctx context.Context, c *bot.Context) error { // any other button
		return c.Answer(ctx, "This button does nothing.")
	})

	// groups runs only in groups; what it does not handle goes on to the handlers after it.
	groups := b.Group(filter.Group())
	groups.OnCommand("rules", bot.ReplyWith("Be kind and stay on topic."))
	b.OnCommand("rules", bot.ReplyWith("Rules apply only in groups."))

	// Handle takes any filter; OnMessage takes every other new message.
	b.Handle(filter.Photo(), bot.ReplyWith("Nice photo!"))
	b.OnMessage(func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "Send /help to see what I can do.")
	})
	return b.Run(ctx) // receives updates until ctx ends
}

// start is a handler written as a named function; c is the current update.
func start(ctx context.Context, c *bot.Context) error {
	return c.Send(ctx, "Hello! Send /help.")
}

// vote sends three buttons; each sends its data back to the bot when pressed.
func vote(ctx context.Context, c *bot.Context) error {
	return c.Send(ctx, "Do you like Go?", bot.Keyboard(models.NewInlineKeyboard(models.NewInlineRow(
		models.NewCallbackButton("Yes", "vote:yes"),
		models.NewCallbackButton("No", "vote:no"),
		models.NewCallbackButton("Maybe", "maybe"),
	))))
}

// reportError gets the errors of handlers; c is nil for errors outside a handler.
func reportError(ctx context.Context, c *bot.Context, err error) {
	log.Print(err)
	if c != nil && c.Chat() != nil {
		_ = c.Send(ctx, "Sorry, something went wrong.")
	}
}
