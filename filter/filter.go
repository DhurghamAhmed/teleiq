// Package filter provides filters for package bot and ways to combine them.
package filter

import (
	"slices"
	"strings"

	"github.com/DhurghamAhmed/teleiq/bot"
)

// Command matches new messages that start with one of the given commands.
func Command(names ...string) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		cmd := c.Command()
		return cmd != "" && slices.ContainsFunc(names, func(name string) bool {
			return strings.EqualFold(strings.TrimPrefix(name, "/"), cmd)
		})
	})
}

// Text matches new messages whose text is exactly text.
func Text(text string) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		m := c.Update().Message
		return m != nil && m.Text != nil && *m.Text == text
	})
}

// Private matches updates from private chats.
func Private() bot.Filter {
	return chatType("private")
}

// Group matches updates from groups and supergroups.
func Group() bot.Filter {
	return chatType("group", "supergroup")
}

// Channel matches updates from channels.
func Channel() bot.Filter {
	return chatType("channel")
}

func chatType(types ...string) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		chat := c.Chat()
		return chat != nil && slices.Contains(types, chat.Type)
	})
}

// ChatID matches updates from the chats with the given IDs.
func ChatID(ids ...int64) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		chat := c.Chat()
		return chat != nil && slices.Contains(ids, chat.ID)
	})
}

// UserID matches updates caused by the users with the given IDs.
func UserID(ids ...int64) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		user := c.Sender()
		return user != nil && slices.Contains(ids, user.ID)
	})
}

// And matches updates that every filter matches.
func And(filters ...bot.Filter) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		for _, f := range filters {
			if !match(f, c) {
				return false
			}
		}
		return true
	})
}

// Or matches updates that any filter matches.
func Or(filters ...bot.Filter) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		return slices.ContainsFunc(filters, func(f bot.Filter) bool { return match(f, c) })
	})
}

// Not matches updates that f does not match.
func Not(f bot.Filter) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool { return !match(f, c) })
}

func match(f bot.Filter, c *bot.Context) bool {
	return f != nil && f.Match(c)
}
