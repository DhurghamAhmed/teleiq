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

// A /cancel command that ends the conversation of the user, or says that there is none: the zero
// State, with an empty Name, means no conversation. Registered before the handlers of the steps, it
// works in the middle of a conversation.
func ExampleHandle_cancel() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	store := fsm.NewMemory(time.Hour)

	cancelConversation := fsm.Handle(store, func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		if s.Name == "" {
			return c.Send(ctx, "There is nothing to cancel.")
		}
		*s = fsm.State{}
		return c.Send(ctx, "Cancelled.")
	})
	b.OnCommand("cancel", cancelConversation)

	ctx := context.Background()
	u := teleiqtest.MessageUpdate(7, "/cancel")
	c := b.NewContext(&u)
	if err := cancelConversation(ctx, c); err != nil {
		log.Fatal(err)
	}
	// The user is now in the middle of a conversation.
	if err := store.Set(ctx, fsm.Key(c), fsm.State{Name: "age", Data: map[string]string{"name": "Ann"}}); err != nil {
		log.Fatal(err)
	}
	if err := cancelConversation(ctx, c); err != nil {
		log.Fatal(err)
	}
	s, _ := store.Get(ctx, fsm.Key(c))
	fmt.Printf("state: %q\n", s.Name)
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// state: ""
	// There is nothing to cancel.
	// Cancelled.
}
