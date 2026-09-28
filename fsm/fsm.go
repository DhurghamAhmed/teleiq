// Package fsm keeps the state of conversations with users.
package fsm

import (
	"context"
	"maps"
	"slices"
	"strconv"

	"github.com/DhurghamAhmed/teleiq/bot"
)

// State is where a conversation is, with the data it has collected so far.
type State struct {
	Name string            // the step of the conversation; empty when there is none
	Data map[string]string // values collected along the way
}

func (s State) clone() State {
	return State{Name: s.Name, Data: maps.Clone(s.Data)}
}

func (s State) isZero() bool {
	return s.Name == "" && len(s.Data) == 0
}

// StateStorage stores the states of conversations.
type StateStorage interface {
	Get(ctx context.Context, key string) (State, error)
	Set(ctx context.Context, key string, s State) error
	Delete(ctx context.Context, key string) error
}

// Key returns the default key of the conversation of an update, as "chat:user".
func Key(c *bot.Context) string {
	var chat, user int64
	if ch := c.Chat(); ch != nil {
		chat = ch.ID
	}
	if u := c.Sender(); u != nil {
		user = u.ID
	}
	return strconv.FormatInt(chat, 10) + ":" + strconv.FormatInt(user, 10)
}

// InState matches updates whose conversation in storage is at one of the steps.
func InState(storage StateStorage, names ...string) bot.Filter {
	return bot.FilterFunc(func(c *bot.Context) bool {
		if storage == nil {
			return false
		}
		s, err := storage.Get(context.Background(), Key(c))
		return err == nil && s.Name != "" && slices.Contains(names, s.Name)
	})
}
