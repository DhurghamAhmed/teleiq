// Command basic checks BOT_TOKEN with getMe and sends a message to CHAT_ID if it is set.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func main() {
	// ctx ends on Ctrl+C, which cancels the requests in progress.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Getenv("BOT_TOKEN"), os.Getenv("CHAT_ID"), log.New(os.Stdout, "", 0))
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token, chat string, out *log.Logger, opts ...teleiq.Option) error {
	if token == "" {
		return errors.New("set BOT_TOKEN to the token @BotFather gave your bot")
	}
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		return err
	}

	me, err := client.GetMe(ctx) // me is the bot itself
	if errors.Is(err, teleiq.ErrUnauthorized) {
		return fmt.Errorf("the Bot API rejected BOT_TOKEN; check it with @BotFather: %w", err)
	}
	if err != nil {
		return err
	}
	out.Printf("Authorized as %s (id %d)", me.FirstName, me.ID)

	if chat == "" {
		out.Print("Set CHAT_ID to send a message as well.")
		return nil
	}
	chatID, err := parseChatID(chat)
	if err != nil {
		return err
	}
	msg, err := client.SendMessage(ctx, teleiq.SendMessageParams{
		ChatID: chatID,
		Text:   "Hello from " + me.FirstName + "!",
	})
	if err != nil {
		return err
	}
	out.Printf("Sent message %d to chat %d", msg.MessageID, msg.Chat.ID)
	return nil
}

// parseChatID reads CHAT_ID: a numeric ID or an @username.
func parseChatID(s string) (models.ChatID, error) {
	if strings.HasPrefix(s, "@") {
		return models.Username(s), nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return models.ChatID{}, fmt.Errorf("CHAT_ID must be a numeric chat ID or an @username, not %q", s)
	}
	return models.ID(id), nil
}
