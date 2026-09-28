package teleiqtest

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/DhurghamAhmed/teleiq/models"
)

// Push makes u available to getUpdates and returns its update_id.
func (s *Server) Push(u models.Update) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdate++
	u.UpdateID = s.lastUpdate
	for _, m := range []*models.Message{u.Message, u.EditedMessage, u.ChannelPost, u.EditedChannelPost,
		u.BusinessMessage, u.EditedBusinessMessage} {
		if m == nil {
			continue
		}
		if m.MessageID == 0 {
			s.lastMessage++
			m.MessageID = s.lastMessage
		}
		if m.Date == 0 {
			m.Date = time.Now().Unix()
		}
	}
	if q := u.CallbackQuery; q != nil && q.ID == "" {
		q.ID = strconv.FormatInt(u.UpdateID, 10)
	}
	if q := u.InlineQuery; q != nil && q.ID == "" {
		q.ID = strconv.FormatInt(u.UpdateID, 10)
	}
	s.updates = append(s.updates, u)
	s.notify()
	return u.UpdateID
}

// WaitHandled waits until the bot has confirmed the update with the given update_id.
func (s *Server) WaitHandled(t testing.TB, updateID int64) {
	t.Helper()
	s.waitFor(t, fmt.Sprintf("update %d to be handled", updateID), func() bool { return s.offset > updateID })
}

// getUpdates answers getUpdates like the Bot API, honoring offset, limit and timeout.
func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request, req Request) {
	offset := int64Of(req.Params["offset"])
	limit := int(int64Of(req.Params["limit"]))
	if limit < 1 || limit > 100 {
		limit = 100
	}
	var expired <-chan time.Time
	if timeout := int64Of(req.Params["timeout"]); timeout > 0 {
		timer := time.NewTimer(time.Duration(timeout) * time.Second)
		defer timer.Stop()
		expired = timer.C
	}
	for {
		s.mu.Lock()
		if offset > 0 {
			s.confirm(offset)
		}
		batch := append([]models.Update{}, s.updates[:min(limit, len(s.updates))]...)
		changed := s.changed
		s.mu.Unlock()
		if len(batch) > 0 || expired == nil {
			writeResult(w, batch)
			return
		}
		select {
		case <-changed:
		case <-expired:
			writeResult(w, batch)
			return
		case <-s.closed:
			writeResult(w, batch)
			return
		case <-r.Context().Done():
			return
		}
	}
}

// confirm forgets the updates below offset; s.mu must be held.
func (s *Server) confirm(offset int64) {
	s.updates = slices.DeleteFunc(s.updates, func(u models.Update) bool { return u.UpdateID < offset })
	if offset > s.offset {
		s.offset = offset
		s.notify()
	}
}

// MessageUpdate returns an update with a text message from user in a private chat.
func MessageUpdate(user int64, text string) models.Update {
	return GroupMessageUpdate(user, user, text)
}

// GroupMessageUpdate returns an update with a new text message from user in chat.
func GroupMessageUpdate(chat, user int64, text string) models.Update {
	return models.Update{Message: &models.Message{
		Chat:     chatOf(chat),
		From:     userOf(user),
		Text:     &text,
		Entities: commandEntity(text),
	}}
}

// CallbackUpdate returns an update with user pressing a button with callback_data data.
func CallbackUpdate(user, messageID int64, data string) models.Update {
	return models.Update{CallbackQuery: &models.CallbackQuery{
		From:         *userOf(user),
		ChatInstance: "teleiqtest",
		Data:         &data,
		Message:      &models.Message{MessageID: messageID, Date: time.Now().Unix(), Chat: chatOf(user), From: &botUser},
	}}
}

// InlineQueryUpdate returns an update with the inline query query from user.
func InlineQueryUpdate(user int64, query string) models.Update {
	return models.Update{InlineQuery: &models.InlineQuery{From: *userOf(user), Query: query, ChatType: ptr("sender")}}
}

// JoinRequestUpdate returns an update with the request of user to join chat.
func JoinRequestUpdate(chat, user int64) models.Update {
	return models.Update{ChatJoinRequest: &models.ChatJoinRequest{Chat: chatOf(chat), From: *userOf(user), UserChatID: user, Date: time.Now().Unix()}}
}

// commandEntity returns the bot_command entity of a text that starts with a command.
func commandEntity(text string) []models.MessageEntity {
	if !strings.HasPrefix(text, "/") {
		return nil
	}
	command := text
	if i := strings.IndexFunc(text, unicode.IsSpace); i >= 0 {
		command = text[:i]
	}
	if len(command) < 2 {
		return nil
	}
	// Telegram measures entities in UTF-16 code units.
	return []models.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(utf16.Encode([]rune(command)))}}
}

func chatOf(id int64) models.Chat {
	switch {
	case id > 0:
		return models.Chat{ID: id, Type: "private", FirstName: &userOf(id).FirstName}
	case id < -1000000000000:
		return models.Chat{ID: id, Type: "supergroup", Title: ptr("Test supergroup")}
	default:
		return models.Chat{ID: id, Type: "group", Title: ptr("Test group")}
	}
}

func userOf(id int64) *models.User {
	return &models.User{ID: id, FirstName: "User" + strconv.FormatInt(id, 10)}
}

func ptr[T any](v T) *T { return &v }
