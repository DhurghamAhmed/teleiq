package bot

import (
	"context"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Context is the update that a handler handles, with the bot that received it.
type Context struct {
	update  *models.Update
	bot     *Bot
	handled bool // a filter matched the update, so its handler ran; Logging reports it
}

// NewContext returns the Context that handlers get for u, such as for tests.
func (b *Bot) NewContext(u *models.Update) *Context {
	if u == nil {
		u = &models.Update{}
	}
	return &Context{update: u, bot: b}
}

// Update returns the update being handled.
func (c *Context) Update() *models.Update { return c.update }

// Client returns the client of the bot, to call the Bot API.
func (c *Context) Client() *teleiq.Client { return c.bot.client }

// Message returns the message of the update, or nil if it has none.
func (c *Context) Message() *models.Message {
	if m := messageOf(c.update); m != nil {
		return m
	}
	if cq := c.update.CallbackQuery; cq != nil {
		if m, ok := cq.Message.(*models.Message); ok {
			return m
		}
	}
	return nil
}

// Chat returns the chat of the update, or nil for updates without one.
func (c *Context) Chat() *models.Chat { return updateChat(c.update) }

// Sender returns the user who caused the update, or nil when there is none.
func (c *Context) Sender() *models.User { return updateSender(c.update) }

// Me returns the bot as getMe described it, or nil before the bot starts.
func (c *Context) Me() *models.User {
	c.bot.mu.RLock()
	defer c.bot.mu.RUnlock()
	if c.bot.me == nil {
		return nil
	}
	me := *c.bot.me // a copy, so that a handler cannot change what the others see
	return &me
}

// Command returns the command of a new message, without its slash and mention.
func (c *Context) Command() string {
	name, _ := c.command()
	return name
}

// Args returns the trimmed text after the command of a new message.
func (c *Context) Args() string {
	_, args := c.command()
	return args
}

func (c *Context) command() (name, args string) {
	m := c.update.Message
	if m == nil || m.Text == nil {
		return "", ""
	}
	name, mention, args, ok := parseCommand(*m.Text)
	if !ok {
		return "", ""
	}
	if mention != "" {
		me := c.Me()
		if me == nil || me.Username == nil || !strings.EqualFold(mention, *me.Username) {
			return "", ""
		}
	}
	return name, args
}

// SendOption changes a message that the bot sends or edits.
type SendOption func(*teleiq.SendMessageParams)

// Reply sends text to the chat of the update and returns the sent message.
func (c *Context) Reply(ctx context.Context, text string, opts ...SendOption) (*models.Message, error) {
	p := teleiq.SendMessageParams{Text: text}
	err := c.fillDestination("Reply", destinationFields{chat: &p.ChatID, thread: &p.MessageThreadID,
		business: &p.BusinessConnectionID, topic: &p.DirectMessagesTopicID})
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&p)
		}
	}
	return c.bot.client.SendMessage(ctx, p)
}
