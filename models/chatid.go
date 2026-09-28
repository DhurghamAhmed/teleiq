package models

import (
	"encoding/json"
	"errors"
	"strconv"
)

// ChatID identifies a chat by its numeric ID or by the @username of a public chat.
type ChatID struct {
	id       int64
	username string
}

// ID returns the ChatID for a numeric chat ID.
func ID(id int64) ChatID {
	return ChatID{id: id}
}

// Username returns the ChatID for the @username of a public chat.
func Username(username string) ChatID {
	return ChatID{username: username}
}

// ID returns the numeric ID of the chat, or 0 when c is a username.
func (c ChatID) ID() int64 {
	return c.id
}

// Username returns the username of the chat, or "" when c is a numeric ID.
func (c ChatID) Username() string {
	return c.username
}

// MarshalJSON encodes a numeric ID as a JSON number and a username as a JSON string.
func (c ChatID) MarshalJSON() ([]byte, error) {
	switch {
	case c.username != "":
		return json.Marshal(c.username)
	case c.id != 0:
		return strconv.AppendInt(nil, c.id, 10), nil
	}
	return nil, errors.New("teleiq: ChatID is not set")
}
