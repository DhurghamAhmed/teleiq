// Command fsm asks for a name and an age after /register; /cancel stops it.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/fsm"
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
	store := fsm.NewMemory(time.Hour) // forgets conversations idle for an hour
	b := bot.New(client)              // b is the bot
	b.Use(bot.Recovery())             // a panic in a handler does not stop the bot

	// register is the conversation; commands come first, so they work mid-way.
	register := fsm.NewConversation(b, store)
	b.OnCommand("cancel", register.End(bot.ReplyWith("Cancelled.")))
	b.OnCommand("register", register.Begin("name", bot.ReplyWith("What is your name?")))
	// s is this user's state: s.Name is the step, s.Data the answers.
	register.Step("name", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		s.Name = "age" // moves to the next question
		s.Set("name", c.Text())
		return c.Send(ctx, "How old are you?")
	})
	register.Step("age", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		age, err := strconv.Atoi(c.Text())
		if err != nil || age < 1 || age > 150 {
			return c.Send(ctx, "Please send your age as a number.") // stays at this step
		}
		name := s.Data["name"]
		*s = fsm.State{} // the empty state ends the conversation
		return c.Send(ctx, fmt.Sprintf("Registered %s, %d years old.", name, age))
	})
	return b.Run(ctx) // receives updates until ctx ends
}
