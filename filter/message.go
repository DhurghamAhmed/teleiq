package filter

import (
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Message matches new messages for which f returns true.
func Message(f func(m *models.Message) bool) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		m := c.Update().Message
		return m != nil && f != nil && f(m)
	})
}

// Reply matches new messages that reply to a message of the same chat.
func Reply() bot.Filter {
	return Message(func(m *models.Message) bool { return m.ReplyToMessage != nil })
}

// Forwarded matches new messages forwarded from another chat or user.
func Forwarded() bot.Filter {
	return Message(func(m *models.Message) bool { return m.ForwardOrigin != nil })
}

// ViaBot matches new messages sent through an inline bot.
func ViaBot() bot.Filter {
	return Message(func(m *models.Message) bool { return m.ViaBot != nil })
}

// FromBot matches new messages sent by a bot.
func FromBot() bot.Filter {
	return Message(func(m *models.Message) bool { return m.From != nil && m.From.IsBot })
}

// AutomaticForward matches channel posts forwarded to their discussion group.
func AutomaticForward() bot.Filter {
	return Message(func(m *models.Message) bool { return m.IsAutomaticForward })
}

// FromOffline matches new messages sent without the sender at hand.
func FromOffline() bot.Filter {
	return Message(func(m *models.Message) bool { return m.IsFromOffline })
}

// InlineKeyboard matches new messages with an inline keyboard.
func InlineKeyboard() bot.Filter {
	return Message(func(m *models.Message) bool { return m.ReplyMarkup != nil })
}

// Regex matches new messages whose text matches re.
func Regex(re *regexp.Regexp) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		text := c.Text()
		return re != nil && text != "" && re.MatchString(text)
	})
}

// Mentioned matches new messages that mention the bot or reply to it.
func Mentioned() bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		m := c.Update().Message
		me := c.Me()
		if m == nil || me == nil {
			return false
		}
		if r := m.ReplyToMessage; r != nil && r.From != nil && r.From.ID == me.ID {
			return true
		}
		return mentions(m.Text, m.Entities, me) || mentions(m.Caption, m.CaptionEntities, me)
	})
}

func mentions(text *string, entities []models.MessageEntity, me *models.User) bool {
	if text == nil || len(entities) == 0 {
		return false
	}
	units := utf16.Encode([]rune(*text)) // Telegram measures entities in UTF-16 code units.
	for _, e := range entities {
		switch {
		case e.Type == "text_mention" && e.User != nil && e.User.ID == me.ID:
			return true
		case e.Type == "mention" && me.Username != nil && e.Offset >= 0 && e.Length > 0 && e.Offset+e.Length <= len(units):
			name := string(utf16.Decode(units[e.Offset : e.Offset+e.Length]))
			if strings.EqualFold(strings.TrimPrefix(name, "@"), *me.Username) {
				return true
			}
		}
	}
	return false
}
