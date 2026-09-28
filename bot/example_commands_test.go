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

// The bot sets its command menu as it starts, right after getMe and before it handles any update.
func ExampleWithCommands() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())

	b := bot.New(client, bot.WithCommands(
		models.BotCommand{Command: "start", Description: "Say hello"},
		models.BotCommand{Command: "help", Description: "List the commands"},
	))
	b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
		defer stop()
		return c.Send(ctx, "Hello!")
	})

	srv.Push(teleiqtest.MessageUpdate(7, "/start"))
	fmt.Println(b.Run(ctx))
	for _, r := range srv.Requests("") {
		fmt.Println(r.Method)
	}
	fmt.Println(srv.Requests("setMyCommands")[0].Params["commands"])
	// Output:
	// <nil>
	// getMe
	// setMyCommands
	// sendMessage
	// [map[command:start description:Say hello] map[command:help description:List the commands]]
}
