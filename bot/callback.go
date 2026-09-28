package bot

import (
	"context"
	"errors"

	"github.com/DhurghamAhmed/teleiq"
)

// OnCallbackPrefix runs h for callback queries whose data starts with prefix.
func (b *Bot) OnCallbackPrefix(prefix string, h Handler) {
	b.Handle(callbackPrefix(prefix), h)
}

// CallbackData returns the data of the pressed button, or "" for other updates.
func (c *Context) CallbackData() string {
	if q := c.update.CallbackQuery; q != nil && q.Data != nil {
		return *q.Data
	}
	return ""
}

// Answer answers the callback query of the update, showing text as a notification.
func (c *Context) Answer(ctx context.Context, text string) error {
	q := c.update.CallbackQuery
	if q == nil {
		return errors.New("bot: Answer: the update is not a callback query")
	}
	p := teleiq.AnswerCallbackQueryParams{CallbackQueryID: q.ID}
	if text != "" {
		p.Text = &text
	}
	err := c.bot.client.AnswerCallbackQuery(ctx, p)
	if errors.Is(err, teleiq.ErrQueryTooOld) {
		return nil
	}
	return err
}
