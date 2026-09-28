package bot_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// exampleBot returns a bot on a fake server, whose requests the examples print.
func exampleBot() (*teleiqtest.Server, *bot.Bot) {
	srv := teleiqtest.NewServer()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	return srv, bot.New(client)
}

// A handler that always sends the same message, here with an inline keyboard. The example runs the
// handler on an update itself, as a test can, instead of starting the bot.
func ExampleReplyWith() {
	srv, b := exampleBot()
	defer srv.Close()

	start := bot.ReplyWith("Hello!", bot.Keyboard(models.NewInlineKeyboard(
		models.NewInlineRow(models.NewURLButton("Help", "https://example.com/help")),
	)))
	b.OnCommand("start", start)

	u := teleiqtest.MessageUpdate(7, "/start")
	fmt.Println(start(context.Background(), b.NewContext(&u)))
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params)
	}
	// Output:
	// <nil>
	// map[chat_id:7 reply_markup:map[inline_keyboard:[[map[text:Help url:https://example.com/help]]]] text:Hello!]
}

// A handler that echoes the text of each message, escaped for Markdown.
func ExampleContext_Send() {
	srv, b := exampleBot()
	defer srv.Close()

	echo := func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "*You said:* "+teleiq.EscapeMarkdown(c.Text()), bot.Markdown())
	}
	b.OnMessage(echo)

	u := teleiqtest.MessageUpdate(7, "1+1=2.")
	fmt.Println(echo(context.Background(), b.NewContext(&u)))
	for _, r := range srv.Requests("sendMessage") {
		fmt.Println(r.Params)
	}
	// Output:
	// <nil>
	// map[chat_id:7 parse_mode:MarkdownV2 text:*You said:* 1\+1\=2\.]
}

// A handler for the buttons whose data starts with "color:": it answers the press, then edits the
// message of the button, even when the answer fails, and returns both errors.
func ExampleBot_OnCallbackPrefix() {
	srv, b := exampleBot()
	defer srv.Close()

	pick := func(ctx context.Context, c *bot.Context) error {
		color := strings.TrimPrefix(c.CallbackData(), "color:")
		answerErr := c.Answer(ctx, "You picked "+color)
		return errors.Join(answerErr, c.Edit(ctx, "Your color: <b>"+color+"</b>", bot.HTML()))
	}
	b.OnCallbackPrefix("color:", pick)

	u := teleiqtest.CallbackUpdate(7, 42, "color:red")
	u.CallbackQuery.ID = "1"
	fmt.Println(pick(context.Background(), b.NewContext(&u)))
	for _, r := range srv.Requests("") {
		fmt.Println(r.Method, r.Params)
	}
	// Output:
	// <nil>
	// answerCallbackQuery map[callback_query_id:1 text:You picked red]
	// editMessageText map[chat_id:7 message_id:42 parse_mode:HTML text:Your color: <b>red</b>]
}
