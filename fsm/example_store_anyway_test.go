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
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// The last step of a registration ends the conversation, tells the user, then sends a badge. The
// badge fails to send, yet the user was told that the registration is over: StoreAnyway stores the
// end of the conversation, and the handler still returns the error for the ErrorHandler.
func ExampleStoreAnyway() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	store := fsm.NewMemory(24 * time.Hour)

	age := fsm.Handle(store, func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		name := s.Data["name"]
		*s = fsm.State{} // the conversation is over
		if err := c.Send(ctx, "Registered "+name+", "+c.Text()+" years old."); err != nil {
			return err // the user was not told, so the conversation stays at this step
		}
		_, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{Photo: models.FileFromBytes("badge.png", []byte("\x89PNG"))})
		return fsm.StoreAnyway(err)
	})
	b.Handle(filter.And(fsm.InState(store, "age"), filter.AnyText()), age)

	ctx := context.Background()
	u := teleiqtest.MessageUpdate(7, "30")
	c := b.NewContext(&u)
	if err := store.Set(ctx, fsm.Key(c), fsm.State{Name: "age", Data: map[string]string{"name": "Ann"}}); err != nil {
		log.Fatal(err)
	}
	srv.Fail("sendPhoto", &teleiq.Error{ErrorCode: 400, Description: "Bad Request: IMAGE_PROCESS_FAILED"})
	fmt.Println(age(ctx, c))
	s, _ := store.Get(ctx, fsm.Key(c))
	fmt.Printf("state: %q\n", s.Name)
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// teleiq: sendPhoto: Bad Request: IMAGE_PROCESS_FAILED (400)
	// state: ""
	// Registered Ann, 30 years old.
}
