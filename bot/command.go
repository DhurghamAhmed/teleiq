package bot

import (
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/DhurghamAhmed/teleiq/models"
)

// WithCommands makes the bot set commands as its command menu as it starts.
func WithCommands(commands ...models.BotCommand) Option {
	return func(c *config) error {
		if len(commands) == 0 {
			return errors.New("bot: WithCommands: no commands")
		}
		c.commands = slices.Clone(commands)
		return nil
	}
}

// parseCommand splits a message text into its command, mention and arguments.
func parseCommand(text string) (name, mention, args string, ok bool) {
	rest, found := strings.CutPrefix(text, "/")
	if !found {
		return "", "", "", false
	}
	token, args := rest, ""
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		token, args = rest[:i], strings.TrimSpace(rest[i:])
	}
	name, mention, _ = strings.Cut(token, "@")
	if !validCommand(name) {
		return "", "", "", false
	}
	return name, mention, args, true
}

// validCommand reports whether name follows the rules of Bot API commands.
func validCommand(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}
