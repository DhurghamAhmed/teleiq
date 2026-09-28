package bot_test

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A bot with a /start command, run on a fake Bot API server from package teleiqtest. A real bot runs
// until Ctrl+C ends its context; this one ends it once its first update has been handled.
func ExampleNew() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())

	b := bot.New(client)
	b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
		defer stop()
		return c.Send(ctx, "Hello, "+c.Sender().FirstName+"!")
	})

	srv.Push(teleiqtest.MessageUpdate(7, "/start"))
	fmt.Println(b.Run(ctx))
	fmt.Println(srv.Requests("sendMessage")[0].Params["text"])
	// Output:
	// <nil>
	// Hello, User7!
}

// HTML formats a message, and html.EscapeString from the standard library escapes the text that is
// not markup, here the name of the user.
func ExampleHTML() {
	srv, b := exampleBot()
	defer srv.Close()

	greet := func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "Hello, <b>"+html.EscapeString(c.Sender().FirstName)+"</b>!", bot.HTML())
	}
	b.OnCommand("start", greet)

	u := teleiqtest.MessageUpdate(7, "/start")
	u.Message.From.FirstName = `Tom & "Jerry"`
	fmt.Println(greet(context.Background(), b.NewContext(&u)))
	sent := srv.Requests("sendMessage")[0].Params
	fmt.Println(sent["parse_mode"], sent["text"])
	// Output:
	// <nil>
	// HTML Hello, <b>Tom &amp; &#34;Jerry&#34;</b>!
}

// A short timeout on a call that the handler can do without. Under flood control, Telegram asks for
// a wait of 30 seconds here; since that does not fit in the timeout, Answer fails at once instead
// of holding the chat. The handler still edits the message, and errors.Join returns the error of
// the answer for the ErrorHandler.
func ExampleContext_Answer() {
	srv, b := exampleBot()
	defer srv.Close()
	wait := 30
	srv.Fail("answerCallbackQuery", &teleiq.Error{
		ErrorCode:   429,
		Description: "Too Many Requests: retry after 30",
		Parameters:  &models.ResponseParameters{RetryAfter: &wait},
	})

	vote := func(ctx context.Context, c *bot.Context) error {
		answerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		answerErr := c.Answer(answerCtx, "Thanks for voting")
		return errors.Join(answerErr, c.Edit(ctx, "Votes: 1"))
	}
	b.OnCallbackPrefix("vote:", vote)

	u := teleiqtest.CallbackUpdate(7, 42, "vote:yes")
	start := time.Now()
	err := vote(context.Background(), b.NewContext(&u))
	fmt.Println("flood control:", errors.Is(err, teleiq.ErrTooManyRequests))
	fmt.Println("edits:", len(srv.Requests("editMessageText")))
	fmt.Println("within a second:", time.Since(start) < time.Second)
	// Output:
	// flood control: true
	// edits: 1
	// within a second: true
}
