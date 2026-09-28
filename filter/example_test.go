package filter_test

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// And combines filters, here to run a /ban handler only in groups and supergroups.
func ExampleAnd() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	banInGroups := filter.And(filter.Group(), filter.Command("ban"))
	b.Handle(banInGroups, bot.ReplyWith("Banned."))

	for _, u := range []models.Update{
		teleiqtest.GroupMessageUpdate(-100, 7, "/ban"),
		teleiqtest.GroupMessageUpdate(-100, 7, "hello"),
		teleiqtest.MessageUpdate(7, "/ban"),
	} {
		fmt.Println(u.Message.Chat.Type, *u.Message.Text, banInGroups.Match(b.NewContext(&u)))
	}
	// Output:
	// group /ban true
	// group hello false
	// private /ban false
}

// AnyCommand matches every command for the bot. Since each update goes to the first handler that
// matches it, AnyCommand registered after the handlers of the known commands gets only the others.
func ExampleAnyCommand() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	b.OnCommand("start", bot.ReplyWith("Hello!"))
	b.Handle(filter.AnyCommand(), bot.ReplyWith("Unknown command. Send /help."))

	for _, text := range []string{"/start", "/stop now", "hello"} {
		u := teleiqtest.MessageUpdate(7, text)
		fmt.Println(text, filter.AnyCommand().Match(b.NewContext(&u)))
	}
	// Output:
	// /start true
	// /stop now true
	// hello false
}

// AnyText matches the new messages with text that a step of a conversation waits for, not
// commands, edits or button presses.
func ExampleAnyText() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	message := teleiqtest.MessageUpdate(7, "Ann")
	command := teleiqtest.MessageUpdate(7, "/help")
	edit := models.Update{EditedMessage: message.Message}
	press := teleiqtest.CallbackUpdate(7, 1, "Ann")
	for _, u := range []models.Update{message, command, edit, press} {
		fmt.Println(filter.AnyText().Match(b.NewContext(&u)))
	}
	// Output:
	// true
	// false
	// false
	// false
}

// CallbackPrefix matches the buttons whose data starts with a prefix, here to combine it with
// another filter.
func ExampleCallbackPrefix() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	colorInPrivate := filter.And(filter.Private(), filter.CallbackPrefix("color:"))

	for _, data := range []string{"color:red", "size:big"} {
		u := teleiqtest.CallbackUpdate(7, 1, data)
		fmt.Println(data, colorInPrivate.Match(b.NewContext(&u)))
	}
	// Output:
	// color:red true
	// size:big false
}

// Message builds a filter about the content of a new message, here the photos whose caption
// carries a hashtag, which Regex does not see since it reads texts only.
func ExampleMessage() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	sale := filter.Message(func(m *models.Message) bool {
		return m.Caption != nil && strings.Contains(*m.Caption, "#sale")
	})

	for _, caption := range []string{"Shoes #sale", "Shoes"} {
		u := models.Update{Message: &models.Message{Chat: models.Chat{ID: 7, Type: "private"},
			Photo: []models.PhotoSize{{FileID: "p"}}, Caption: &caption}}
		fmt.Println(caption, sale.Match(b.NewContext(&u)))
	}
	// Output:
	// Shoes #sale true
	// Shoes false
}

// Regex matches the texts of new messages by a pattern, here commands that start with "!", which
// are not commands for Telegram.
func ExampleRegex() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)
	ban := filter.Regex(regexp.MustCompile(`(?i)^!ban\b`))

	for _, text := range []string{"!ban 42", "!BAN", "please !ban"} {
		u := teleiqtest.GroupMessageUpdate(-100, 7, text)
		fmt.Println(text, ban.Match(b.NewContext(&u)))
	}
	// Output:
	// !ban 42 true
	// !BAN true
	// please !ban false
}

// Service matches the messages that tell of an event in a chat, such as a member who joined or
// left, rather than carry what a user sent.
func ExampleService() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	joined := teleiqtest.GroupMessageUpdate(-100, 7, "")
	joined.Message.Text, joined.Message.NewChatMembers = nil, []models.User{{ID: 8, FirstName: "Bob"}}
	left := teleiqtest.GroupMessageUpdate(-100, 7, "")
	left.Message.Text, left.Message.LeftChatMember = nil, &models.User{ID: 8, FirstName: "Bob"}
	text := teleiqtest.GroupMessageUpdate(-100, 7, "hello")
	for _, u := range []models.Update{joined, left, text} {
		fmt.Println(filter.Service().Match(b.NewContext(&u)), filter.NewChatMembers().Match(b.NewContext(&u)))
	}
	// Output:
	// true true
	// true false
	// false false
}
