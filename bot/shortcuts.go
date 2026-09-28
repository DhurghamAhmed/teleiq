package bot

import (
	"context"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Send sends text to the chat of the update, like Reply without the sent message.
func (c *Context) Send(ctx context.Context, text string, opts ...SendOption) error {
	_, err := c.Reply(ctx, text, opts...)
	return err
}

// ReplyWith returns a handler that sends text to the chat of each update.
func ReplyWith(text string, opts ...SendOption) Handler {
	return func(ctx context.Context, c *Context) error { return c.Send(ctx, text, opts...) }
}

// HTML makes the text be parsed as HTML, with teleiq.ParseModeHTML.
func HTML() SendOption {
	return func(p *teleiq.SendMessageParams) { p.ParseMode = teleiq.Ptr(teleiq.ParseModeHTML) }
}

// Markdown makes the text be parsed as Telegram's MarkdownV2.
func Markdown() SendOption {
	return func(p *teleiq.SendMessageParams) { p.ParseMode = teleiq.Ptr(teleiq.ParseModeMarkdown) }
}

// Keyboard attaches the reply markup m to the message.
func Keyboard(m models.ReplyMarkup) SendOption {
	return func(p *teleiq.SendMessageParams) { p.ReplyMarkup = m }
}

// Text returns the text of a new message, or "" for other updates.
func (c *Context) Text() string {
	if m := c.update.Message; m != nil && m.Text != nil {
		return *m.Text
	}
	return ""
}
