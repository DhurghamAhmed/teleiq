package fsm_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/fsm"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A Conversation asks for a name and a city in turn, with a /cancel that works at any step. The
// bot runs on a fake Bot API server and stops after the last answer.
func ExampleConversation() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	b := bot.New(client)

	signup := fsm.NewConversation(b, fsm.NewMemory(time.Hour))
	b.OnCommand("cancel", signup.End(bot.ReplyWith("Cancelled.")))
	b.OnCommand("signup", signup.Begin("name", bot.ReplyWith("Your name?")))
	signup.Step("name", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		s.Name = "city"
		s.Set("name", c.Text())
		return c.Send(ctx, "Your city?")
	})
	signup.Step("city", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		defer stop() // the last answer of this example
		name := s.Data["name"]
		*s = fsm.State{}
		return c.Send(ctx, name+" from "+c.Text()+", welcome!")
	})

	for _, text := range []string{"/signup", "Ann", "Basra"} {
		srv.Push(teleiqtest.MessageUpdate(7, text))
	}
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// Your name?
	// Your city?
	// Ann from Basra, welcome!
}
