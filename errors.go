package teleiq

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/DhurghamAhmed/teleiq/models"

	"github.com/DhurghamAhmed/teleiq/internal/sanitize"
)

var (
	// ErrInvalidToken reports a bot token that is clearly malformed.
	ErrInvalidToken = errors.New("teleiq: invalid bot token")

	// ErrUnauthorized matches an *Error for a token the Bot API rejects (401).
	ErrUnauthorized = errors.New("teleiq: unauthorized")

	// ErrForbidden matches an *Error for an action the bot may not take (403).
	ErrForbidden = errors.New("teleiq: forbidden")

	// ErrConflict matches an *Error for a competing getUpdates call or webhook (409).
	ErrConflict = errors.New("teleiq: conflict")

	// ErrTooManyRequests matches an *Error for flood control (429).
	ErrTooManyRequests = errors.New("teleiq: too many requests")

	// ErrMethodNotFound matches an *Error for a method the server does not know (404).
	ErrMethodNotFound = errors.New("teleiq: method not found")
)

// The errors below match an *Error by its code and the text of its description.
var (
	// ErrBotBlocked matches an *Error for a user who blocked the bot (403).
	ErrBotBlocked = errors.New("teleiq: bot blocked by the user")

	// ErrUserDeactivated matches an *Error for a user whose account was deleted (403).
	ErrUserDeactivated = errors.New("teleiq: user deactivated")

	// ErrNotStarted matches an *Error for a user who never started the bot (403).
	ErrNotStarted = errors.New("teleiq: user has not started the bot")

	// ErrBotKicked matches an *Error for a chat that removed the bot (403).
	ErrBotKicked = errors.New("teleiq: bot kicked from the chat")

	// ErrNotMember matches an *Error for a chat that the bot is not a member of (403).
	ErrNotMember = errors.New("teleiq: bot is not a member of the chat")

	// ErrBotToBot matches an *Error for a message from the bot to another bot (403).
	ErrBotToBot = errors.New("teleiq: bots cannot message bots")

	// ErrNotEnoughRights matches an *Error for an action that needs rights the bot lacks.
	ErrNotEnoughRights = errors.New("teleiq: not enough rights")

	// ErrMemberIsAdmin matches an *Error for acting on a chat administrator (400).
	ErrMemberIsAdmin = errors.New("teleiq: member is an administrator")

	// ErrWrongChatType matches an *Error for a method the chat type does not allow (400).
	ErrWrongChatType = errors.New("teleiq: method not available in this chat")

	// ErrChatNotFound matches an *Error for an unknown chat (400).
	ErrChatNotFound = errors.New("teleiq: chat not found")

	// ErrUserNotFound matches an *Error for an unknown user (400).
	ErrUserNotFound = errors.New("teleiq: user not found")

	// ErrGroupMigrated matches an *Error for a group that became a supergroup (400).
	ErrGroupMigrated = errors.New("teleiq: group migrated to a supergroup")

	// ErrMessageNotFound matches an *Error for a missing or inaccessible message (400).
	ErrMessageNotFound = errors.New("teleiq: message not found")

	// ErrMessageNotModified matches an *Error for an edit that changes nothing (400).
	ErrMessageNotModified = errors.New("teleiq: message is not modified")

	// ErrMessageCantBeEdited matches an *Error for a message the bot may not edit (400).
	ErrMessageCantBeEdited = errors.New("teleiq: message can't be edited")

	// ErrMessageCantBeDeleted matches an *Error for a message the bot can't delete (400).
	ErrMessageCantBeDeleted = errors.New("teleiq: message can't be deleted")

	// ErrEmptyText matches an *Error for a message without text (400).
	ErrEmptyText = errors.New("teleiq: message text is empty")

	// ErrMessageTooLong matches an *Error for a text or caption that is too long (400).
	ErrMessageTooLong = errors.New("teleiq: message is too long")

	// ErrCantParseEntities matches an *Error for malformed HTML or Markdown (400).
	ErrCantParseEntities = errors.New("teleiq: can't parse entities")

	// ErrButtonDataInvalid matches an *Error for invalid button callback data (400).
	ErrButtonDataInvalid = errors.New("teleiq: button data invalid")

	// ErrReplyMarkupTooLong matches an *Error for a keyboard that is too large (400).
	ErrReplyMarkupTooLong = errors.New("teleiq: reply markup is too long")

	// ErrQueryTooOld matches an *Error for a query answered too late (400).
	ErrQueryTooOld = errors.New("teleiq: query is too old")

	// ErrInvalidFileID matches an *Error for a malformed or unknown file_id (400).
	ErrInvalidFileID = errors.New("teleiq: invalid file_id")

	// ErrFileTooBig matches an *Error for a file too big for a bot to download (400).
	ErrFileTooBig = errors.New("teleiq: file is too big")

	// ErrAnotherInstance matches an *Error for a second poller using the token (409).
	ErrAnotherInstance = errors.New("teleiq: another instance polls with the token")

	// ErrWebhookActive matches an *Error for getUpdates while a webhook is set (409).
	ErrWebhookActive = errors.New("teleiq: webhook is active")
)

// description holds the code and texts by which an *Error matches a named error.
type description struct {
	code  int
	texts []string
}

var descriptions = map[error]description{
	ErrBotBlocked:      {http.StatusForbidden, []string{"bot was blocked by the user"}},
	ErrUserDeactivated: {http.StatusForbidden, []string{"user is deactivated"}},
	ErrNotStarted:      {http.StatusForbidden, []string{"bot can't initiate conversation with a user"}},
	ErrBotKicked:       {http.StatusForbidden, []string{"bot was kicked from"}},
	ErrNotMember:       {http.StatusForbidden, []string{"bot is not a member of"}},
	ErrBotToBot:        {http.StatusForbidden, []string{"bot can't send messages to the bot", "bots can't send messages to bots"}},
	// Telegram reports missing rights as 400 or 403, depending on the method.
	ErrNotEnoughRights: {0, []string{"not enough rights", "have no rights", "chat_admin_required", "need administrator rights", "member list is inaccessible"}},
	ErrMemberIsAdmin:   {http.StatusBadRequest, []string{"user is an administrator of the chat", "can't remove chat owner"}},
	ErrWrongChatType: {http.StatusBadRequest, []string{"method is available only in supergroups", "method is available only for supergroups",
		"method is available for supergroup and channel chats only", "can't ban members in private chats"}},
	ErrChatNotFound:  {http.StatusBadRequest, []string{"chat not found"}},
	ErrUserNotFound:  {http.StatusBadRequest, []string{"user not found"}},
	ErrGroupMigrated: {http.StatusBadRequest, []string{"group chat was upgraded to a supergroup chat"}},
	ErrMessageNotFound: {http.StatusBadRequest, []string{"message to edit not found", "message to delete not found",
		"message to forward not found", "message to copy not found", "message to pin not found", "message to unpin not found",
		"message to react not found", "message to be replied not found", "reply message not found", "message not found",
		"invalid inline message identifier"}},
	ErrMessageNotModified:   {http.StatusBadRequest, []string{"message is not modified"}},
	ErrMessageCantBeEdited:  {http.StatusBadRequest, []string{"message can't be edited"}},
	ErrMessageCantBeDeleted: {http.StatusBadRequest, []string{"message can't be deleted"}},
	ErrEmptyText:            {http.StatusBadRequest, []string{"message text is empty", "text must be non-empty"}},
	ErrMessageTooLong:       {http.StatusBadRequest, []string{"message is too long", "message caption is too long"}},
	ErrCantParseEntities:    {http.StatusBadRequest, []string{"can't parse entities"}},
	ErrButtonDataInvalid:    {http.StatusBadRequest, []string{"button_data_invalid"}},
	ErrReplyMarkupTooLong:   {http.StatusBadRequest, []string{"reply markup is too long"}},
	ErrQueryTooOld:          {http.StatusBadRequest, []string{"query is too old"}},
	ErrInvalidFileID:        {http.StatusBadRequest, []string{"invalid file_id", "wrong remote file identifier", "wrong file identifier"}},
	ErrFileTooBig:           {http.StatusBadRequest, []string{"file is too big"}},
	ErrAnotherInstance:      {http.StatusConflict, []string{"terminated by other getupdates request"}},
	ErrWebhookActive:        {http.StatusConflict, []string{"can't use getupdates method while webhook is active"}},
}

// Error is an error returned by the Bot API.
type Error struct {
	ErrorCode   int
	Description string
	Parameters  *models.ResponseParameters
	Method      string
}

// Error formats e as "teleiq: <method>: <description> (<code>)".
func (e *Error) Error() string {
	msg := "teleiq: "
	if e.Method != "" {
		msg += e.Method + ": "
	}
	if e.Description != "" {
		msg += e.Description
	} else {
		msg += "error"
	}
	// The description comes from the server, which could echo a token back.
	return sanitize.String(msg + " (" + strconv.Itoa(e.ErrorCode) + ")")
}

// Is reports whether e matches target, one of the errors of this package.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.ErrorCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.ErrorCode == http.StatusForbidden
	case ErrConflict:
		return e.ErrorCode == http.StatusConflict
	case ErrTooManyRequests:
		return e.ErrorCode == http.StatusTooManyRequests
	case ErrMethodNotFound:
		return e.ErrorCode == http.StatusNotFound
	case ErrGroupMigrated:
		if e.Parameters != nil && e.Parameters.MigrateToChatID != nil {
			return true
		}
	}
	d, ok := descriptions[target]
	if !ok || d.code != 0 && e.ErrorCode != d.code {
		return false
	}
	text := strings.ToLower(e.Description)
	return slices.ContainsFunc(d.texts, func(t string) bool { return strings.Contains(text, t) })
}
