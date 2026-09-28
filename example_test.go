package teleiq_test

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A bot is created with the token that @BotFather gave it, as in teleiq.NewClient(os.Getenv("BOT_TOKEN")).
// Here WithBaseURL points the client at a fake Bot API server from package teleiqtest, as a test does.
func ExampleNewClient() {
	srv := teleiqtest.NewServer()
	defer srv.Close()

	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	msg, err := client.SendMessage(context.Background(), teleiq.SendMessageParams{
		ChatID: models.ID(123456789),
		Text:   "Hello",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(msg.Chat.ID, *msg.Text)
	// Output: 123456789 Hello
}

// errors.Is tells the failures of the Bot API apart by name, here while sending news to users
// who may have blocked the bot or deleted their account since they subscribed.
func ExampleError_Is() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL), teleiq.WithRetryPolicy(teleiq.Backoff{MaxAttempts: 1}))
	if err != nil {
		log.Fatal(err)
	}
	srv.Fail("sendMessage", &teleiq.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"})

	_, err = client.SendMessage(context.Background(), teleiq.SendMessageParams{ChatID: models.ID(7), Text: "News"})
	switch {
	case errors.Is(err, teleiq.ErrBotBlocked), errors.Is(err, teleiq.ErrUserDeactivated):
		fmt.Println("unsubscribe user 7")
	case errors.Is(err, teleiq.ErrTooManyRequests):
		fmt.Println("send later")
	case err != nil:
		fmt.Println("failed:", err)
	}
	// Output: unsubscribe user 7
}

// Text sent with ParseModeMarkdown is escaped with EscapeMarkdown, except the markup itself.
func ExampleEscapeMarkdown() {
	fmt.Println("*Welcome,* " + teleiq.EscapeMarkdown("Ann_Lee (admin)") + "\\!")
	// Output: *Welcome,* Ann\_Lee \(admin\)\!
}

// Text sent with ParseModeHTML is escaped with html.EscapeString from the standard library.
func Example_parseModeHTML() {
	fmt.Println("<b>Welcome,</b> " + html.EscapeString(`<Tom & "Jerry">`))
	// Output: <b>Welcome,</b> &lt;Tom &amp; &#34;Jerry&#34;&gt;
}

// A bot that broadcasts paces its messages to the free limits of Telegram: about 30 messages a
// second in all, one a second in a chat and 20 a minute in a group. Chat 7 is sent to twice, so its
// second message waits a second; the others go at once.
func ExampleNewRateLimiter() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL),
		teleiq.WithRateLimiter(teleiq.NewRateLimiter(30, time.Second, 3*time.Second)))
	if err != nil {
		log.Fatal(err)
	}

	start := time.Now()
	for _, chat := range []int64{7, 8, 9, 7} {
		if _, err := client.SendMessage(context.Background(), teleiq.SendMessageParams{ChatID: models.ID(chat), Text: "News"}); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println(len(srv.Requests("sendMessage")), "messages in", time.Since(start).Round(time.Second))
	// Output: 4 messages in 1s
}

// WithDefaults sets the parse mode once for every message, and a call that sets its own keeps it.
func ExampleWithDefaults() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL),
		teleiq.WithDefaults(teleiq.Defaults{ParseMode: teleiq.ParseModeHTML}))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	for _, p := range []teleiq.SendMessageParams{
		{ChatID: models.ID(7), Text: "<b>Hello</b>"},
		{ChatID: models.ID(7), Text: "2 < 3", ParseMode: teleiq.Ptr("")}, // plain text
	} {
		if _, err := client.SendMessage(ctx, p); err != nil {
			log.Fatal(err)
		}
	}
	for _, r := range srv.Requests("sendMessage") {
		fmt.Printf("%q %q\n", r.Params["text"], r.Params["parse_mode"])
	}
	// Output:
	// "<b>Hello</b>" "HTML"
	// "2 < 3" ""
}
