package bot

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// target is the message that the message shortcuts act on.
type target struct {
	chat     models.ChatID
	id       int64
	business *string
	message  *models.Message // nil for a message too old to be accessible
}

// target returns the message of the update or of its callback query.
func (c *Context) target(method string) (target, error) {
	if m := c.Message(); m != nil {
		return target{chat: models.ID(m.Chat.ID), id: m.MessageID, business: m.BusinessConnectionID, message: m}, nil
	}
	if q := c.update.CallbackQuery; q != nil {
		if m, ok := q.Message.(*models.InaccessibleMessage); ok {
			return target{chat: models.ID(m.Chat.ID), id: m.MessageID}, nil
		}
	}
	return target{}, fmt.Errorf("bot: %s: the update has no message in a chat", method)
}

// Delete deletes the message of the update or of its callback query.
func (c *Context) Delete(ctx context.Context) error {
	t, err := c.target("Delete")
	if err != nil {
		return err
	}
	if t.business != nil {
		return c.bot.client.DeleteBusinessMessages(ctx, teleiq.DeleteBusinessMessagesParams{
			BusinessConnectionID: *t.business, MessageIDs: []int64{t.id}})
	}
	err = c.bot.client.DeleteMessage(ctx, teleiq.DeleteMessageParams{ChatID: t.chat, MessageID: t.id})
	if errors.Is(err, teleiq.ErrMessageNotFound) {
		return nil
	}
	return err
}

// Forward forwards the message of the update or of its callback query to a chat.
func (c *Context) Forward(ctx context.Context, to models.ChatID) (*models.Message, error) {
	t, err := c.target("Forward")
	if err != nil {
		return nil, err
	}
	return c.bot.client.ForwardMessage(ctx, teleiq.ForwardMessageParams{ChatID: to, FromChatID: t.chat, MessageID: t.id})
}

// Copy copies the message of the update or of its callback query to a chat.
func (c *Context) Copy(ctx context.Context, to models.ChatID) (*models.MessageId, error) {
	t, err := c.target("Copy")
	if err != nil {
		return nil, err
	}
	return c.bot.client.CopyMessage(ctx, teleiq.CopyMessageParams{ChatID: to, FromChatID: t.chat, MessageID: t.id})
}

// Pin pins the message of the update or of its callback query in its chat.
func (c *Context) Pin(ctx context.Context) error {
	t, err := c.target("Pin")
	if err != nil {
		return err
	}
	return c.bot.client.PinChatMessage(ctx, teleiq.PinChatMessageParams{ChatID: t.chat, MessageID: t.id, BusinessConnectionID: t.business})
}

// Unpin unpins the message of the update or of its callback query.
func (c *Context) Unpin(ctx context.Context) error {
	t, err := c.target("Unpin")
	if err != nil {
		return err
	}
	return c.bot.client.UnpinChatMessage(ctx, teleiq.UnpinChatMessageParams{ChatID: t.chat, MessageID: &t.id, BusinessConnectionID: t.business})
}

// React sets the reaction of the bot to the message of the update to emoji.
func (c *Context) React(ctx context.Context, emoji string) error {
	t, err := c.target("React")
	if err != nil {
		return err
	}
	p := teleiq.SetMessageReactionParams{ChatID: t.chat, MessageID: t.id, Reaction: []models.ReactionType{}}
	if emoji != "" {
		p.Reaction = []models.ReactionType{&models.ReactionTypeEmoji{Emoji: emoji}}
	}
	return c.bot.client.SetMessageReaction(ctx, p)
}

// Download writes the file of the message of the update to dst.
func (c *Context) Download(ctx context.Context, dst io.Writer) error {
	t, err := c.target("Download")
	if err != nil {
		return err
	}
	id := fileOf(t.message)
	if id == "" {
		return errors.New("bot: Download: the message has no file")
	}
	f, err := c.bot.client.GetFile(ctx, teleiq.GetFileParams{FileID: id})
	if err != nil {
		return err
	}
	if f.FilePath == nil {
		return errors.New("bot: Download: Telegram gave no path for the file")
	}
	return c.bot.client.Download(ctx, *f.FilePath, dst)
}

// fileOf returns the ID of the file of m, or "" when it has none.
func fileOf(m *models.Message) string {
	switch {
	case m == nil:
		return ""
	case m.LivePhoto != nil:
		return m.LivePhoto.FileID
	case len(m.Photo) > 0:
		largest := m.Photo[0]
		for _, p := range m.Photo[1:] {
			if p.Width*p.Height > largest.Width*largest.Height {
				largest = p
			}
		}
		return largest.FileID
	case m.Animation != nil:
		return m.Animation.FileID
	case m.Document != nil:
		return m.Document.FileID
	case m.Audio != nil:
		return m.Audio.FileID
	case m.Video != nil:
		return m.Video.FileID
	case m.VideoNote != nil:
		return m.VideoNote.FileID
	case m.Voice != nil:
		return m.Voice.FileID
	case m.Sticker != nil:
		return m.Sticker.FileID
	}
	return ""
}
