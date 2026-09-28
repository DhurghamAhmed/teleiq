// Command callback shows color buttons on /menu and answers the one pressed.
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
	b.OnCommand("menu", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; each button sends its data, such as "color:red".
		return c.Send(ctx, "Pick a color:", bot.Keyboard(models.NewInlineKeyboard(models.NewInlineRow(
			models.NewCallbackButton("Red", "color:red"),
			models.NewCallbackButton("Green", "color:green"),
			models.NewCallbackButton("Blue", "color:blue"),
		))))
	})
	// Runs for the buttons whose data starts with "color:".
	b.OnCallbackPrefix("color:", func(ctx context.Context, c *bot.Context) error {
		color := strings.TrimPrefix(c.CallbackData(), "color:")
		// Every press must be answered; the edit runs even if the answer fails.
		answerErr := c.Answer(ctx, "You picked "+color)
		return errors.Join(answerErr, c.Edit(ctx, "Your color: "+color))
	})
	return b.Run(ctx) // receives updates until ctx ends
}
