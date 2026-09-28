package bot_test

import (
	"context"
	"fmt"
	"log"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// SendPhoto sends to the chat of the update, here a /cat command written in a forum topic, and
// keeps the topic.
func ExampleContext_SendPhoto() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	cat := func(ctx context.Context, c *bot.Context) error {
		_, err := c.SendPhoto(ctx, teleiq.SendPhotoParams{
			Photo:   models.FileFromBytes("cat.jpg", []byte("a picture of a cat")),
			Caption: teleiq.Ptr("Meow"),
		})
		return err
	}
	b.OnCommand("cat", cat)

	u := teleiqtest.GroupMessageUpdate(-1001234567890, 7, "/cat")
	u.Message.MessageThreadID, u.Message.IsTopicMessage = teleiq.Ptr(int64(3)), true
	if err := cat(context.Background(), b.NewContext(&u)); err != nil {
		log.Fatal(err)
	}

	r := srv.Requests("sendPhoto")[0]
	fmt.Println(int64(r.Params["chat_id"].(float64)), r.Params["message_thread_id"], r.Params["caption"], r.Uploads["photo"].Name)
	// Output: -1001234567890 3 Meow cat.jpg
}
