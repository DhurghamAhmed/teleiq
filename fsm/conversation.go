package fsm

import (
	"context"
	"errors"

	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/filter"
)

// Conversation adds the handlers of a multi-step conversation to a bot or a group.
type Conversation struct {
	router  interface{ Handle(bot.Filter, bot.Handler) }
	storage StateStorage
}

// NewConversation returns a Conversation for r that keeps its states in storage.
func NewConversation(r interface{ Handle(bot.Filter, bot.Handler) }, storage StateStorage) *Conversation {
	return &Conversation{router: r, storage: storage}
}

// Begin returns a handler that starts the conversation at step and then runs h.
func (cv *Conversation) Begin(step string, h bot.Handler) bot.Handler {
	return Handle(cv.storage, func(ctx context.Context, c *bot.Context, s *State) error {
		switch {
		case step == "":
			return errors.New("fsm: Begin: empty step")
		case h == nil:
			return errors.New("fsm: Begin: nil handler")
		}
		*s = State{Name: step}
		return h(ctx, c)
	})
}

// End returns a handler that ends the conversation of the update and then runs h.
func (cv *Conversation) End(h bot.Handler) bot.Handler {
	return Handle(cv.storage, func(ctx context.Context, c *bot.Context, s *State) error {
		if h == nil {
			return errors.New("fsm: End: nil handler")
		}
		*s = State{}
		return h(ctx, c)
	})
}

// Step runs h for the text answers of the conversations at the step name.
func (cv *Conversation) Step(name string, h func(ctx context.Context, c *bot.Context, s *State) error) {
	cv.router.Handle(filter.And(InState(cv.storage, name), filter.AnyText()), Handle(cv.storage, h))
}

// Set sets the value of key in the data of s, making the data first if s has none.
func (s *State) Set(key, value string) {
	if s.Data == nil {
		s.Data = map[string]string{}
	}
	s.Data[key] = value
}
