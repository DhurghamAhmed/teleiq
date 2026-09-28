package teleiqtest_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// The server records every call, and Fail makes a method fail as Telegram would, here as if the
// user had blocked the bot, until Reset.
func Example() {
	srv := teleiqtest.NewServer()
	defer srv.Close()

	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	m, err := client.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "Hello"})
	fmt.Println(*m.Text, err)

	srv.Fail("sendMessage", &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"})
	_, err = client.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "Are you there?"})
	fmt.Println(errors.Is(err, teleiq.ErrForbidden))

	srv.Reset("sendMessage")
	_, err = client.SendMessage(ctx, teleiq.SendMessageParams{ChatID: models.ID(7), Text: "Welcome back"})
	fmt.Println(err)

	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["chat_id"], r.Params["text"])
	}
	// Output:
	// Hello <nil>
	// true
	// <nil>
	// 7 Hello
	// 7 Are you there?
	// 7 Welcome back
}

// An inline query as a user would type it; Push gives it an ID, which the bot answers with.
func ExampleInlineQueryUpdate() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}

	srv.Push(teleiqtest.InlineQueryUpdate(7, "cats"))
	updates, err := client.GetUpdates(context.Background(), teleiq.GetUpdatesParams{})
	if err != nil {
		log.Fatal(err)
	}
	q := updates[0].InlineQuery
	fmt.Println(q.ID, q.From.FirstName, q.Query, *q.ChatType)
	// Output: 1 User7 cats sender
}
