package filter

import (
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Photo matches new messages with a photo, other than a live photo.
func Photo() bot.Filter {
	return Message(func(m *models.Message) bool { return len(m.Photo) > 0 && m.LivePhoto == nil })
}

// LivePhoto matches new messages with a live photo.
func LivePhoto() bot.Filter {
	return Message(func(m *models.Message) bool { return m.LivePhoto != nil })
}

// Document matches new messages with a document other than an animation.
func Document() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Document != nil && m.Animation == nil })
}

// Animation matches new messages with an animation, a GIF or a video without sound.
func Animation() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Animation != nil })
}

// Audio matches new messages with an audio file, meant to be treated as music.
func Audio() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Audio != nil })
}

// Video matches new messages with a video.
func Video() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Video != nil })
}

// Voice matches new messages with a voice message.
func Voice() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Voice != nil })
}

// VideoNote matches new messages with a video note, a round video message.
func VideoNote() bot.Filter {
	return Message(func(m *models.Message) bool { return m.VideoNote != nil })
}

// Sticker matches new messages with a sticker.
func Sticker() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Sticker != nil })
}

// Media matches new messages with a file that bot.Context.Download downloads.
func Media() bot.Filter {
	return Message(func(m *models.Message) bool {
		return m.Animation != nil || m.Audio != nil || m.Document != nil || len(m.Photo) > 0 || m.LivePhoto != nil ||
			m.Sticker != nil || m.Video != nil || m.VideoNote != nil || m.Voice != nil
	})
}

// MediaGroup matches new messages that are part of an album.
func MediaGroup() bot.Filter {
	return Message(func(m *models.Message) bool { return m.MediaGroupID != nil })
}

// Caption matches new messages with a caption.
func Caption() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Caption != nil && *m.Caption != "" })
}

// Spoiler matches new messages whose media is covered by a spoiler.
func Spoiler() bot.Filter {
	return Message(func(m *models.Message) bool { return m.HasMediaSpoiler })
}

// Contact matches new messages with a shared contact.
func Contact() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Contact != nil })
}

// Location matches new messages with a shared location, other than a venue.
func Location() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Location != nil && m.Venue == nil })
}

// Venue matches new messages with a venue.
func Venue() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Venue != nil })
}

// Poll matches new messages with a poll.
func Poll() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Poll != nil })
}

// Dice matches new messages with an animated emoji that shows a random value.
func Dice() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Dice != nil })
}

// Game matches new messages with a game.
func Game() bot.Filter {
	return Message(func(m *models.Message) bool { return m.Game != nil })
}
