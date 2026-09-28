// Command shop shows products as buttons on /start and takes the order pressed.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

type product struct {
	ID   int64
	Name string
}

var products = []product{{1, "Tea"}, {2, "Coffee"}, {3, "Juice"}, {4, "Water"}, {5, "Milk"}}

// buy is the data of the buttons: "buy:<product>:<quantity>".
var buy = bot.NewCallback("buy")

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
	// Every message is HTML, so the texts can use <b> and <code>.
	opts = append(opts, teleiq.WithDefaults(teleiq.Defaults{ParseMode: teleiq.ParseModeHTML}))
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	b := bot.New(client)  // b is the bot
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; c.Send replies in its chat.
		grid := models.NewInlineGrid(2) // rows of two buttons
		for _, p := range products {
			grid.Add(buy.Button(p.Name, p.ID, 1)) // data "buy:<id>:1": the product, quantity 1
		}
		grid.Row(models.NewCallbackButton("« Close", "close")) // alone in the last row
		return c.Send(ctx, "<b>Pick a product:</b>", bot.Keyboard(grid.Markup()))
	})
	// buy is also a filter that matches the presses of its buttons.
	b.Handle(buy, func(ctx context.Context, c *bot.Context) error {
		fields := buy.Unpack(c.CallbackData())
		id, quantity := fields.Int64(0), fields.Int(1) // the product and the quantity
		// Err reports data that does not fit, such as a stale button.
		if fields.Err() != nil {
			return c.Answer(ctx, "This button is out of date.")
		}
		// Answer stops the button's loading; Edit replaces the menu text.
		answerErr := c.Answer(ctx, "Added to your order.")
		return errors.Join(answerErr, c.Edit(ctx, fmt.Sprintf("You ordered <b>%d</b> of product <code>%d</code>.", quantity, id)))
	})
	b.OnCallbackPrefix("close", func(ctx context.Context, c *bot.Context) error {
		return errors.Join(c.Answer(ctx, ""), c.Delete(ctx))
	})
	return b.Run(ctx) // receives updates until ctx ends
}
