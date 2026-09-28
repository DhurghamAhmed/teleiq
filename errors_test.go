package teleiq

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"full", &Error{ErrorCode: 400, Description: "Bad Request: chat not found", Method: "sendMessage"}, "teleiq: sendMessage: Bad Request: chat not found (400)"},
		{"no method", &Error{ErrorCode: 401, Description: "Unauthorized"}, "teleiq: Unauthorized (401)"},
		{"no description", &Error{ErrorCode: 500, Method: "getMe"}, "teleiq: getMe: error (500)"},
		{"token in description", &Error{ErrorCode: 404, Description: "Not Found: /bot" + testToken + "/getMe", Method: "getMe"}, "teleiq: getMe: Not Found: /bot<redacted>/getMe (404)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorIs(t *testing.T) {
	sentinels := []error{ErrInvalidToken, ErrUnauthorized, ErrForbidden, ErrConflict, ErrTooManyRequests, ErrMethodNotFound}
	tests := []struct {
		name string
		code int
		want error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, ErrForbidden},
		{"conflict", http.StatusConflict, ErrConflict},
		{"too many requests", http.StatusTooManyRequests, ErrTooManyRequests},
		{"method not found", http.StatusNotFound, ErrMethodNotFound},
		{"bad request", http.StatusBadRequest, nil},
		{"server error", http.StatusInternalServerError, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr := &Error{ErrorCode: tt.code, Description: http.StatusText(tt.code), Method: "sendMessage"}
			for _, err := range []error{apiErr, fmt.Errorf("broadcast: %w", apiErr)} {
				for _, s := range sentinels {
					if got, want := errors.Is(err, s), s == tt.want; got != want {
						t.Errorf("errors.Is(%v, %v) = %v, want %v", err, s, got, want)
					}
				}
			}
		})
	}
}

func TestErrorAs(t *testing.T) {
	apiErr := &Error{
		ErrorCode:   http.StatusTooManyRequests,
		Description: "Too Many Requests: retry after 35",
		Parameters:  &models.ResponseParameters{RetryAfter: Ptr(35)},
		Method:      "sendMessage",
	}
	tests := []struct {
		name string
		err  error
		want *Error
	}{
		{"direct", apiErr, apiErr},
		{"wrapped", fmt.Errorf("broadcast: %w", apiErr), apiErr},
		{"wrapped twice", fmt.Errorf("job 7: %w", fmt.Errorf("broadcast: %w", apiErr)), apiErr},
		{"other error", io.ErrUnexpectedEOF, nil},
		{"sentinel", ErrTooManyRequests, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *Error
			ok := errors.As(tt.err, &got)
			if ok != (tt.want != nil) || got != tt.want {
				t.Fatalf("errors.As(%v) = %v, %v; want %v", tt.err, got, ok, tt.want)
			}
			if ok && *got.Parameters.RetryAfter != 35 {
				t.Errorf("RetryAfter = %d, want 35", *got.Parameters.RetryAfter)
			}
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	sentinels := []error{ErrInvalidToken, ErrUnauthorized, ErrForbidden, ErrConflict, ErrTooManyRequests, ErrMethodNotFound}
	for err := range descriptions {
		sentinels = append(sentinels, err)
	}
	for i, a := range sentinels {
		if a == nil || !strings.HasPrefix(a.Error(), "teleiq: ") {
			t.Errorf("sentinel %d = %v, want a non-nil error prefixed with \"teleiq: \"", i, a)
		}
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("errors.Is(%v, %v) = true, want sentinels to be distinct", a, b)
			}
		}
	}
}

func TestResponseParametersJSON(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantMigrate *int64
		wantRetry   *int
		wantOut     string
	}{
		{"retry after", `{"retry_after":5}`, nil, Ptr(5), `{"retry_after":5}`},
		{"migrate beyond float precision", `{"migrate_to_chat_id":-9007199254740993}`, Ptr(int64(-9007199254740993)), nil, `{"migrate_to_chat_id":-9007199254740993}`},
		{"empty", `{}`, nil, nil, `{}`},
		{"unknown field ignored", `{"retry_after":1,"future_field":{"x":1}}`, nil, Ptr(1), `{"retry_after":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p models.ResponseParameters
			if err := json.Unmarshal([]byte(tt.in), &p); err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", tt.in, err)
			}
			if !equalPtr(p.MigrateToChatID, tt.wantMigrate) || !equalPtr(p.RetryAfter, tt.wantRetry) {
				t.Errorf("Unmarshal(%s) = %+v", tt.in, p)
			}
			out, err := json.Marshal(p)
			if err != nil || string(out) != tt.wantOut {
				t.Errorf("Marshal = %s, %v; want %s", out, err, tt.wantOut)
			}
		})
	}
}

func equalPtr[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// TestNamedErrors checks each named error against the descriptions Telegram sends. Those marked
// seen were returned by the Bot API itself on 2026-09-28; the others cannot be caused without a
// visible effect, such as a user blocking the bot.
func TestNamedErrors(t *testing.T) {
	migrated := &models.ResponseParameters{MigrateToChatID: Ptr(int64(-1001234567890))}
	tests := []struct {
		code        int
		description string
		parameters  *models.ResponseParameters
		want        error
		seen        bool
	}{
		{403, "Forbidden: bot was blocked by the user", nil, ErrBotBlocked, false},
		{403, "Forbidden: user is deactivated", nil, ErrUserDeactivated, false},
		{403, "Forbidden: bot can't initiate conversation with a user", nil, ErrNotStarted, false},
		{403, "Forbidden: bot was kicked from the group chat", nil, ErrBotKicked, false},
		{403, "Forbidden: bot was kicked from the supergroup chat", nil, ErrBotKicked, false},
		{403, "Forbidden: bot was kicked from the channel chat", nil, ErrBotKicked, false},
		{403, "Forbidden: bot is not a member of the channel chat", nil, ErrNotMember, true},
		{403, "Forbidden: bot is not a member of the supergroup chat", nil, ErrNotMember, false},
		{403, "Forbidden: the bot can't send messages to the bot", nil, ErrBotToBot, true},
		{403, "Forbidden: bots can't send messages to bots", nil, ErrBotToBot, false},
		{403, "Forbidden: not enough rights to send text messages to the chat", nil, ErrNotEnoughRights, false},
		{400, "Bad Request: not enough rights to restrict/unrestrict chat member", nil, ErrNotEnoughRights, false},
		{400, "Bad Request: have no rights to send a message", nil, ErrNotEnoughRights, false},
		{400, "Bad Request: CHAT_ADMIN_REQUIRED", nil, ErrNotEnoughRights, false},
		{400, "Bad Request: member list is inaccessible", nil, ErrNotEnoughRights, true},
		{400, "Bad Request: user is an administrator of the chat", nil, ErrMemberIsAdmin, false},
		{400, "Bad Request: can't remove chat owner", nil, ErrMemberIsAdmin, false},
		{400, "Bad Request: method is available only in supergroups", nil, ErrWrongChatType, true},
		{400, "Bad Request: can't ban members in private chats", nil, ErrWrongChatType, true},
		{400, "Bad Request: chat not found", nil, ErrChatNotFound, true},
		{400, "Bad Request: user not found", nil, ErrUserNotFound, false},
		{400, "Bad Request: group chat was upgraded to a supergroup chat", migrated, ErrGroupMigrated, false},
		{400, "Bad Request: message to edit not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to delete not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to forward not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to copy not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to react not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to unpin not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message to be replied not found", nil, ErrMessageNotFound, true},
		{400, "Bad Request: invalid inline message identifier specified", nil, ErrMessageNotFound, true},
		{400, "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message", nil, ErrMessageNotModified, true},
		{400, "Bad Request: message can't be edited", nil, ErrMessageCantBeEdited, false},
		{400, "Bad Request: message can't be deleted", nil, ErrMessageCantBeDeleted, false},
		{400, "Bad Request: message can't be deleted for everyone", nil, ErrMessageCantBeDeleted, false},
		{400, "Bad Request: message text is empty", nil, ErrEmptyText, true},
		{400, "Bad Request: message is too long", nil, ErrMessageTooLong, true},
		{400, "Bad Request: message caption is too long", nil, ErrMessageTooLong, false},
		{400, `Bad Request: can't parse entities: Can't find end tag corresponding to start tag "b"`, nil, ErrCantParseEntities, true},
		{400, `Bad Request: can't parse entities: Character '.' is reserved and must be escaped with the preceding '\'`, nil, ErrCantParseEntities, true},
		{400, "Bad Request: BUTTON_DATA_INVALID", nil, ErrButtonDataInvalid, true},
		{400, "Bad Request: reply markup is too long", nil, ErrReplyMarkupTooLong, true},
		{400, "Bad Request: query is too old and response timeout expired or query ID is invalid", nil, ErrQueryTooOld, true},
		{400, "Bad Request: invalid file_id", nil, ErrInvalidFileID, true},
		{400, "Bad Request: wrong remote file identifier specified: can't unserialize it. Wrong last symbol", nil, ErrInvalidFileID, true},
		{400, "Bad Request: wrong file identifier/HTTP URL specified", nil, ErrInvalidFileID, false},
		{400, "Bad Request: file is too big", nil, ErrFileTooBig, false},
		{409, "Conflict: terminated by other getUpdates request; make sure that only one bot instance is running", nil, ErrAnotherInstance, true},
		{409, "Conflict: can't use getUpdates method while webhook is active; use deleteWebhook to delete the webhook first", nil, ErrWebhookActive, false},
		// Neither a known description nor the code that Telegram sends with it.
		{400, "Bad Request: something new", nil, nil, false},
		{500, "Internal Server Error: message is not modified", nil, nil, false},
		{403, "Bad Request: chat not found", nil, nil, false},
		// A migrated group is told by its parameters even if the text changes.
		{400, "Bad Request: some other wording", migrated, ErrGroupMigrated, false},
	}
	named := make([]error, 0, len(descriptions))
	covered := map[error]bool{}
	for err := range descriptions {
		named = append(named, err)
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			apiErr := &Error{ErrorCode: tt.code, Description: tt.description, Parameters: tt.parameters, Method: "sendMessage"}
			for _, err := range []error{apiErr, fmt.Errorf("broadcast: %w", apiErr)} {
				for _, n := range named {
					if got, want := errors.Is(err, n), n == tt.want; got != want {
						t.Errorf("errors.Is(%q, %v) = %t, want %t", tt.description, n, got, want)
					}
				}
			}
		})
		covered[tt.want] = true
	}
	for _, n := range named {
		if !covered[n] {
			t.Errorf("no description tests %v", n)
		}
	}
}
