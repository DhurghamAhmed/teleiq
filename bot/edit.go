package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// edited is the message whose button sent the callback query, as the edit methods name it.
type edited struct {
	chat     models.ChatID
	id       *int64
	inline   *string
	business *string
}

// edited returns the message whose button sent the callback query of the update.
func (c *Context) edited(method string) (edited, error) {
	q := c.update.CallbackQuery
	if q == nil {
		return edited{}, fmt.Errorf("bot: %s: the update is not a callback query", method)
	}
	if q.InlineMessageID != nil {
		return edited{inline: q.InlineMessageID}, nil
	}
	switch m := q.Message.(type) {
	case *models.Message:
		return edited{chat: models.ID(m.Chat.ID), id: &m.MessageID, business: m.BusinessConnectionID}, nil
	case *models.InaccessibleMessage:
		return edited{chat: models.ID(m.Chat.ID), id: &m.MessageID}, nil
	}
	return edited{}, fmt.Errorf("bot: %s: the callback query has no message", method)
}

// editOptions applies opts as Send would and returns the inline keyboard they set.
func editOptions(method, text string, opts []SendOption) (teleiq.SendMessageParams, *models.InlineKeyboardMarkup, error) {
	s := teleiq.SendMessageParams{Text: text}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	if s.ReplyMarkup == nil {
		return s, nil, nil
	}
	kb, ok := s.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok {
		return s, nil, fmt.Errorf("bot: %s: a message can be edited only with an inline keyboard", method)
	}
	return s, kb, nil
}

// notModified turns the error of an edit that changes nothing into nil.
func notModified(err error) error {
	if errors.Is(err, teleiq.ErrMessageNotModified) {
		return nil
	}
	return err
}

// Edit replaces the text of the message whose button was pressed.
func (c *Context) Edit(ctx context.Context, text string, opts ...SendOption) error {
	e, err := c.edited("Edit")
	if err != nil {
		return err
	}
	s, kb, err := editOptions("Edit", text, opts)
	if err != nil {
		return err
	}
	_, err = c.bot.client.EditMessageText(ctx, teleiq.EditMessageTextParams{ChatID: e.chat, MessageID: e.id,
		InlineMessageID: e.inline, BusinessConnectionID: e.business, Text: &s.Text, ParseMode: s.ParseMode,
		Entities: s.Entities, LinkPreviewOptions: s.LinkPreviewOptions, ReplyMarkup: kb})
	return notModified(err)
}

// EditCaption replaces the caption of the message whose button was pressed.
func (c *Context) EditCaption(ctx context.Context, caption string, opts ...SendOption) error {
	e, err := c.edited("EditCaption")
	if err != nil {
		return err
	}
	s, kb, err := editOptions("EditCaption", caption, opts)
	if err != nil {
		return err
	}
	_, err = c.bot.client.EditMessageCaption(ctx, teleiq.EditMessageCaptionParams{ChatID: e.chat, MessageID: e.id,
		InlineMessageID: e.inline, BusinessConnectionID: e.business, Caption: &s.Text, ParseMode: s.ParseMode,
		CaptionEntities: s.Entities, ReplyMarkup: kb})
	return notModified(err)
}

// EditKeyboard sets kb as the buttons of the message whose button was pressed.
func (c *Context) EditKeyboard(ctx context.Context, kb *models.InlineKeyboardMarkup) error {
	e, err := c.edited("EditKeyboard")
	if err != nil {
		return err
	}
	_, err = c.bot.client.EditMessageReplyMarkup(ctx, teleiq.EditMessageReplyMarkupParams{ChatID: e.chat,
		MessageID: e.id, InlineMessageID: e.inline, BusinessConnectionID: e.business, ReplyMarkup: kb})
	return notModified(err)
}
