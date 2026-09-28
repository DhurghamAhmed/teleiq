package bot_test

import (
	"context"
	"fmt"
	"log"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// Delete deletes the message of the update, here the service messages that tell a group of the
// members who joined or left.
func ExampleContext_Delete() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	clean := func(ctx context.Context, c *bot.Context) error { return c.Delete(ctx) }
	b.Handle(filter.Or(filter.NewChatMembers(), filter.LeftChatMember()), clean)

	joined := teleiqtest.GroupMessageUpdate(-1001234567890, 7, "")
	joined.Message.MessageID, joined.Message.Text = 42, nil
	joined.Message.NewChatMembers = []models.User{{ID: 7, FirstName: "Ann"}}
	if err := clean(context.Background(), b.NewContext(&joined)); err != nil {
		log.Fatal(err)
	}

	r := srv.Requests("deleteMessage")[0]
	fmt.Println(int64(r.Params["chat_id"].(float64)), r.Params["message_id"])
	// Output: -1001234567890 42
}

// Ban bans a user from the chat of the update, here the sender of the message that a /ban
// command answers.
func ExampleContext_Ban() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	ban := func(ctx context.Context, c *bot.Context) error {
		r := c.Message().ReplyToMessage
		if r == nil || r.From == nil {
			return c.Send(ctx, "Answer a message with /ban.")
		}
		return c.Ban(ctx, r.From.ID)
	}
	b.Handle(filter.And(filter.Group(), filter.Command("ban")), ban)

	u := teleiqtest.GroupMessageUpdate(-1001234567890, 7, "/ban")
	u.Message.ReplyToMessage = &models.Message{From: &models.User{ID: 8, FirstName: "Spammer"}}
	if err := ban(context.Background(), b.NewContext(&u)); err != nil {
		log.Fatal(err)
	}

	r := srv.Requests("banChatMember")[0]
	fmt.Println(int64(r.Params["chat_id"].(float64)), r.Params["user_id"])
	// Output: -1001234567890 8
}

// OnJoinRequest handles the requests to join a chat, which Approve or Decline answers.
func ExampleBot_OnJoinRequest() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	approve := func(ctx context.Context, c *bot.Context) error { return c.Approve(ctx) }
	b.OnJoinRequest(approve)

	u := teleiqtest.JoinRequestUpdate(-1001234567890, 8)
	if err := approve(context.Background(), b.NewContext(&u)); err != nil {
		log.Fatal(err)
	}

	r := srv.Requests("approveChatJoinRequest")[0]
	fmt.Println(int64(r.Params["chat_id"].(float64)), r.Params["user_id"])
	// Output: -1001234567890 8
}
