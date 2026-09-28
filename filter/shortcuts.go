package filter

import (
	"strings"

	"github.com/DhurghamAhmed/teleiq/bot"
)

// AnyCommand matches new messages that start with a command for the bot.
func AnyCommand() bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool { return c.Command() != "" })
}

// AnyText matches new text messages that do not start with a command for the bot.
func AnyText() bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool { return c.Text() != "" && c.Command() == "" })
}

// CallbackPrefix matches callback queries whose data starts with prefix.
func CallbackPrefix(prefix string) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		q := c.Update().CallbackQuery
		return q != nil && q.Data != nil && strings.HasPrefix(*q.Data, prefix)
	})
}
