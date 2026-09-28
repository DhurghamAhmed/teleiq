package bot_test

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// An inline bot answers the text that a user types after its username with results to pick from,
// here the text in bold, and a button to its private chat when there is no text yet.
func ExampleBot_OnInlineQuery() {
	srv, b := exampleBot()
	defer srv.Close()

	bold := func(ctx context.Context, c *bot.Context) error {
		text := strings.TrimSpace(c.Update().InlineQuery.Query)
		if text == "" {
			return c.AnswerInline(ctx, nil, bot.StartButton("How to use", "help"))
		}
		a := bot.Article("bold", "Bold", "<b>"+html.EscapeString(text)+"</b>", bot.HTML())
		a.Description = &text
		return c.AnswerInline(ctx, []models.InlineQueryResult{a}, bot.CacheTime(3600))
	}
	b.OnInlineQuery(bold)

	for _, query := range []string{"Tom & Jerry", ""} {
		u := teleiqtest.InlineQueryUpdate(7, query)
		u.InlineQuery.ID = "q" + strconv.Itoa(len(query))
		if err := bold(context.Background(), b.NewContext(&u)); err != nil {
			log.Fatal(err)
		}
	}
	for _, r := range srv.Requests("answerInlineQuery") {
		fmt.Println(r.Params)
	}
	// Output:
	// map[cache_time:3600 inline_query_id:q11 results:[map[description:Tom & Jerry id:bold input_message_content:map[message_text:<b>Tom &amp; Jerry</b> parse_mode:HTML] title:Bold type:article]]]
	// map[button:map[start_parameter:help text:How to use] inline_query_id:q0 results:[]]
}

// Results come in pages: the answer gives the offset of the next page, and the Telegram app asks for
// it with that Offset when the user scrolls to the end.
func ExampleContext_AnswerInline() {
	srv, b := exampleBot()
	defer srv.Close()
	var numbers []string
	for i := range 25 {
		numbers = append(numbers, strconv.Itoa(i))
	}

	const page = 10
	search := func(ctx context.Context, c *bot.Context) error {
		start, _ := strconv.Atoi(c.Update().InlineQuery.Offset)
		start = min(start, len(numbers))
		end := min(start+page, len(numbers))
		var results []models.InlineQueryResult
		for _, n := range numbers[start:end] {
			results = append(results, bot.Article(n, "Number "+n, n))
		}
		next := ""
		if end < len(numbers) {
			next = strconv.Itoa(end)
		}
		return c.AnswerInline(ctx, results, bot.NextOffset(next))
	}

	for _, offset := range []string{"", "10", "20"} {
		u := teleiqtest.InlineQueryUpdate(7, "numbers")
		u.InlineQuery.ID, u.InlineQuery.Offset = "q", offset
		if err := search(context.Background(), b.NewContext(&u)); err != nil {
			log.Fatal(err)
		}
	}
	for _, r := range srv.Requests("answerInlineQuery") {
		fmt.Printf("%d results, next offset %q\n", len(r.Params["results"].([]any)), r.Params["next_offset"])
	}
	// Output:
	// 10 results, next offset "10"
	// 10 results, next offset "20"
	// 5 results, next offset ""
}

// An article sends a message with the options of Send, such as a parse mode and an inline keyboard.
func ExampleArticle() {
	a := bot.Article("1", "Greeting", "<b>Hello!</b>", bot.HTML(), bot.Keyboard(models.NewInlineKeyboard(
		models.NewInlineRow(models.NewURLButton("Visit", "https://example.com")),
	)))
	a.Description = teleiq.Ptr("A bold hello")

	content := a.InputMessageContent.(*models.InputTextMessageContent)
	fmt.Println(a.ID, a.Title, *a.Description)
	fmt.Println(content.MessageText, *content.ParseMode)
	fmt.Println(a.ReplyMarkup.InlineKeyboard[0][0].Text)
	// Output:
	// 1 Greeting A bold hello
	// <b>Hello!</b> HTML
	// Visit
}
