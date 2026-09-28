package fsm

import (
	"context"
	"errors"
	"maps"

	"github.com/DhurghamAhmed/teleiq/bot"
)

// Handle returns a handler that loads the conversation state, runs h and stores it.
func Handle(storage StateStorage, h func(ctx context.Context, c *bot.Context, s *State) error) bot.Handler {
	return func(ctx context.Context, c *bot.Context) error {
		switch {
		case storage == nil:
			return errors.New("fsm: Handle: nil storage")
		case h == nil:
			return errors.New("fsm: Handle: nil handler")
		}
		key := Key(c)
		s, err := storage.Get(ctx, key)
		if err != nil {
			return err
		}
		// A copy, because h may change the map of s in place.
		before := s.clone()
		err = h(ctx, c, &s)
		var anyway *storeAnyway
		if err != nil && !errors.As(err, &anyway) {
			return err
		}
		var stored error
		switch {
		case s.Name == before.Name && maps.Equal(s.Data, before.Data):
		case s.isZero():
			stored = storage.Delete(ctx, key)
		default:
			stored = storage.Set(ctx, key, s)
		}
		switch {
		case stored == nil:
			return err
		case err == nil:
			return stored
		}
		return errors.Join(err, stored)
	}
}

// StoreAnyway marks err so that Handle still stores the state and returns err.
func StoreAnyway(err error) error {
	if err == nil {
		return nil
	}
	return &storeAnyway{err}
}

type storeAnyway struct{ err error }

func (e *storeAnyway) Error() string { return e.err.Error() }
func (e *storeAnyway) Unwrap() error { return e.err }
