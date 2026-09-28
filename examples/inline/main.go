// Command inline answers "@bot text" in any chat with the text in bold, italic or code.
package main

import (
	"context"
	"html"
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
	b.OnCommand("start", bot.ReplyWith("Type my username and a text in any chat, then pick a style."))
	// Runs when a user types after the bot's username; c is the update.
	b.OnInlineQuery(func(ctx context.Context, c *bot.Context) error {
		text := strings.TrimSpace(c.Update().InlineQuery.Query) // "hello" for "@my_bot hello"
		if text == "" {
			// No text yet: a button sends "/start help" to the bot.
			return c.AnswerInline(ctx, nil, bot.StartButton("How to use", "help"))
		}
		var results []models.InlineQueryResult
		for _, style := range styles {
			// Escaped, so a < or & that the user typed is not read as HTML.
			article := bot.Article(style.id, style.title, "<"+style.tag+">"+html.EscapeString(text)+"</"+style.tag+">", bot.HTML())
			article.Description = &text // shown under the title
			results = append(results, article)
		}
		// The results depend only on the text, so Telegram may share them for an hour.
		return c.AnswerInline(ctx, results, bot.CacheTime(3600))
	})
	return b.Run(ctx) // receives updates until ctx ends
}

// styles are the results offered: the id, title and HTML tag of each.
var styles = []struct{ id, title, tag string }{{"bold", "Bold", "b"}, {"italic", "Italic", "i"}, {"code", "Code", "code"}}
