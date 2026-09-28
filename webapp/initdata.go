package webapp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// InitData is the validated data that Telegram gave a Mini App.
type InitData struct {
	QueryID                string        // the query ID for answerWebAppQuery
	ChatJoinRequestQueryID string        // for answerChatJoinRequestQuery
	User                   *User         // the user who opened the Mini App
	Receiver               *User         // the chat partner, for the attachment menu
	Chat                   *Chat         // the chat of the attachment menu or join request
	ChatType               string        // the type of the chat
	ChatInstance           string        // identifies the chat the Mini App was opened from
	StartParam             string        // the startattach or startapp parameter of the link
	CanSendAfter           time.Duration // how long before answerWebAppQuery can send a message
	AuthDate               time.Time     // when the Mini App was opened
	Hash                   string
	Signature              string
}

// User is a user or a bot in the data of a Mini App.
type User struct {
	ID                    int64  `json:"id"`
	IsBot                 bool   `json:"is_bot"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	Username              string `json:"username"`
	LanguageCode          string `json:"language_code"`
	IsPremium             bool   `json:"is_premium"`
	AddedToAttachmentMenu bool   `json:"added_to_attachment_menu"`
	AllowsWriteToPM       bool   `json:"allows_write_to_pm"`
	PhotoURL              string `json:"photo_url"`
}

// Chat is a group, supergroup or channel in the data of a Mini App.
type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
	PhotoURL string `json:"photo_url"`
}

// decode builds the InitData of checked fields and checks their age.
func decode(fields map[string]string, maxAge time.Duration) (*InitData, error) {
	seconds, err := strconv.ParseInt(fields["auth_date"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: no auth_date", ErrInvalid)
	}
	d := &InitData{
		QueryID:                fields["query_id"],
		ChatJoinRequestQueryID: fields["chat_join_request_query_id"],
		ChatType:               fields["chat_type"],
		ChatInstance:           fields["chat_instance"],
		StartParam:             fields["start_param"],
		AuthDate:               time.Unix(seconds, 0),
		Hash:                   fields["hash"],
		Signature:              fields["signature"],
	}
	switch age := time.Since(d.AuthDate); {
	case age > maxAge:
		return nil, ErrExpired
	case age < -futureSkew:
		return nil, fmt.Errorf("%w: auth_date is in the future", ErrInvalid)
	}
	if s, ok := fields["can_send_after"]; ok {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: can_send_after is not a number", ErrInvalid)
		}
		d.CanSendAfter = time.Duration(n) * time.Second
	}
	for name, dst := range map[string]any{"user": &d.User, "receiver": &d.Receiver, "chat": &d.Chat} {
		if s, ok := fields[name]; ok {
			if err := json.Unmarshal([]byte(s), dst); err != nil {
				return nil, fmt.Errorf("%w: %s is not a JSON object", ErrInvalid, name)
			}
		}
	}
	return d, nil
}
