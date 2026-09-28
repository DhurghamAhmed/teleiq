// Command filters answers each kind of message with the filter that matched it.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"unicode/utf8"

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

// number matches a text made only of digits.
var number = regexp.MustCompile(`^\d+$`)

// long is a filter of its own: new messages with more than 100 characters of text.
var long = filter.Message(func(m *models.Message) bool {
	return m.Text != nil && utf8.RuneCountInString(*m.Text) > 100
})

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, opts ...teleiq.Option) error {
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}
	b := bot.New(client)  // b is the bot
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot

	// Each message goes to the first handler below whose filter matches it.
	b.Handle(filter.Command("start", "help"), bot.ReplyWith("Send me a photo, a voice message, a number or any text."))
	b.Handle(filter.Photo(), bot.ReplyWith("Nice photo!"))
	b.Handle(filter.Or(filter.Voice(), filter.Audio()), bot.ReplyWith("I got your audio."))
	b.Handle(filter.Text("hi"), bot.ReplyWith("Hello!"))
	b.Handle(filter.Regex(number), bot.ReplyWith("That is a number."))
	b.Handle(filter.And(filter.Reply(), filter.Not(filter.Private())), bot.ReplyWith("You replied to someone in a group."))
	b.Handle(long, bot.ReplyWith("That is a long message."))
	b.Handle(filter.AnyText(), func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "You said: "+c.Text()) // c is the current update
	})
	// Anything else, such as a sticker or a file.
	b.OnMessage(bot.ReplyWith("No filter above matches this kind of message."))
	return b.Run(ctx) // receives updates until ctx ends
}
