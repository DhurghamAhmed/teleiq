package bot

import (
	"context"
	"errors"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// OnInlineQuery runs h for the inline queries that users send to the bot.
func (b *Bot) OnInlineQuery(h Handler) {
	b.Handle(onInlineQuery, h)
}

// InlineOption changes the answer that AnswerInline sends.
type InlineOption func(*teleiq.AnswerInlineQueryParams)

// AnswerInline answers the inline query of the update with results.
func (c *Context) AnswerInline(ctx context.Context, results []models.InlineQueryResult, opts ...InlineOption) error {
	q := c.update.InlineQuery
	if q == nil {
		return errors.New("bot: AnswerInline: the update is not an inline query")
	}
	p := teleiq.AnswerInlineQueryParams{Results: results}
	for _, opt := range opts {
		if opt != nil {
			opt(&p)
		}
	}
	p.InlineQueryID = q.ID
	if p.Results == nil {
		p.Results = []models.InlineQueryResult{}
	}
	err := c.bot.client.AnswerInlineQuery(ctx, p)
	if errors.Is(err, teleiq.ErrQueryTooOld) {
		return nil
	}
	return err
}

// CacheTime sets how many seconds Telegram may cache the answer.
func CacheTime(seconds int) InlineOption {
	return func(p *teleiq.AnswerInlineQueryParams) { p.CacheTime = &seconds }
}

// Personal makes Telegram cache the answer only for the user who sent the query.
func Personal() InlineOption {
	return func(p *teleiq.AnswerInlineQueryParams) { p.IsPersonal = teleiq.Ptr(true) }
}

// NextOffset sets the offset of the query that asks for more results.
func NextOffset(offset string) InlineOption {
	return func(p *teleiq.AnswerInlineQueryParams) { p.NextOffset = &offset }
}

// StartButton shows a button above the results that starts the bot with parameter.
func StartButton(text, parameter string) InlineOption {
	return func(p *teleiq.AnswerInlineQueryParams) {
		p.Button = &models.InlineQueryResultsButton{Text: text, StartParameter: &parameter}
	}
}

// Article returns an inline query result that shows title and sends text.
func Article(id, title, text string, opts ...SendOption) *models.InlineQueryResultArticle {
	s := teleiq.SendMessageParams{Text: text}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	a := &models.InlineQueryResultArticle{ID: id, Title: title, InputMessageContent: &models.InputTextMessageContent{
		MessageText:        s.Text,
		ParseMode:          s.ParseMode,
		Entities:           s.Entities,
		LinkPreviewOptions: s.LinkPreviewOptions,
	}}
	if kb, ok := s.ReplyMarkup.(*models.InlineKeyboardMarkup); ok {
		a.ReplyMarkup = kb
	}
	return a
}
