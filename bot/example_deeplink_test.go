package bot_test

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A deep link, here https://t.me/test_bot?start=ref-7, opens the private chat with the bot, and
// pressing Start sends it "/start ref-7": Args is the value. Anyone can type /start with another
// value, so the handler checks it like any input.
func ExampleContext_Args() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	me, err := client.GetMe(ctx)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	invite := func(ctx context.Context, c *bot.Context) error {
		link := "https://t.me/" + *me.Username + "?start=ref-" + strconv.FormatInt(c.Sender().ID, 10)
		return c.Send(ctx, "Invite your friends with "+link)
	}
	start := func(ctx context.Context, c *bot.Context) error {
		ref, ok := strings.CutPrefix(c.Args(), "ref-")
		id, err := strconv.ParseInt(ref, 10, 64)
		if !ok || err != nil {
			return c.Send(ctx, "Welcome!")
		}
		return c.Send(ctx, fmt.Sprintf("Welcome! User %d invited you.", id))
	}
	b.OnCommand("invite", invite)
	b.OnCommand("start", start)

	for _, m := range []struct {
		h    bot.Handler
		user int64
		text string
	}{{invite, 7, "/invite"}, {start, 8, "/start ref-7"}, {start, 9, "/start"}, {start, 10, "/start ref-me"}} {
		u := teleiqtest.MessageUpdate(m.user, m.text)
		if err := m.h(ctx, b.NewContext(&u)); err != nil {
			log.Fatal(err)
		}
	}
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params["text"])
	}
	// Output:
	// Invite your friends with https://t.me/test_bot?start=ref-7
	// Welcome! User 7 invited you.
	// Welcome!
	// Welcome!
}
