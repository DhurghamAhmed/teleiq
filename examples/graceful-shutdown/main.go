// Command graceful-shutdown lets a slow /work finish when Ctrl+C stops the bot.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
)

func main() {
	// ctx ends on Ctrl+C or SIGTERM, which starts the shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv("BOT_TOKEN"), 10*time.Second)
	stop()
	if err != nil {
		log.Fatal(err)
	}
	log.Print("stopped cleanly")
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, work time.Duration, opts ...teleiq.Option) error {
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	// b is the bot; on shutdown it waits up to twice the work.
	b := bot.New(client, bot.WithShutdownTimeout(2*work))
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	b.OnCommand("work", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; c.Send replies in its chat.
		if err := c.Send(ctx, "Working..."); err != nil {
			return err
		}
		// ctx stays live during shutdown, so the handler can stop early.
		select {
		case <-time.After(work):
		case <-ctx.Done():
			return ctx.Err()
		}
		return c.Send(ctx, "Done.")
	})

	err = b.Run(ctx) // receives updates until ctx ends, then finishes the work
	if errors.Is(err, bot.ErrShutdownTimeout) {
		return fmt.Errorf("the work in progress did not finish in time; Telegram will send it again: %w", err)
	}
	return err
}
