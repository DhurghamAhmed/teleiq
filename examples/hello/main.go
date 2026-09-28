// Command hello greets /start and repeats the text of every other message.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
)

func main() {
	// ctx ends on Ctrl+C, which stops the bot.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := teleiq.NewClient(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client) // b is the bot
	b.OnCommand("start", bot.ReplyWith("Hello! Send me a message and I will repeat it."))
	// c is the current message; c.Send replies in its chat.
	b.OnMessage(func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "You said: "+c.Text())
	})
	if err := b.Run(ctx); err != nil { // receives updates until ctx ends
		log.Fatal(err)
	}
}
