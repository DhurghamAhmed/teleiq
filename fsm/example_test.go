package fsm_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/fsm"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A conversation that asks for a name, then greets the user and ends. The example runs each handler
// on an update itself, as a test can, instead of starting the bot.
func ExampleHandle() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	store := fsm.NewMemory(24 * time.Hour)

	register := fsm.Handle(store, func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		*s = fsm.State{Name: "name"}
		return c.Send(ctx, "What is your name?")
	})
	greet := fsm.Handle(store, func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		*s = fsm.State{} // the conversation is over
		return c.Send(ctx, "Nice to meet you, "+c.Text()+"!")
	})
	b.OnCommand("register", register)
	b.Handle(filter.And(fsm.InState(store, "name"), filter.AnyText()), greet)

	ctx := context.Background()
	for _, step := range []struct {
		h    bot.Handler
		text string
	}{{register, "/register"}, {greet, "Ann"}} {
		u := teleiqtest.MessageUpdate(7, step.text)
		c := b.NewContext(&u)
		if err := step.h(ctx, c); err != nil {
			log.Fatal(err)
		}
		s, _ := store.Get(ctx, fsm.Key(c))
		fmt.Printf("state after %q: %q\n", step.text, s.Name)
	}
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// state after "/register": "name"
	// state after "Ann": ""
	// What is your name?
	// Nice to meet you, Ann!
}
