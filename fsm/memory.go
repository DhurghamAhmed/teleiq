package fsm

import (
	"context"
	"sync"
	"time"
)

// Memory is a StateStorage that keeps states in memory.
type Memory struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	states map[string]entry
	writes int // Sets since the last sweep of expired states
}

type entry struct {
	state   State
	expires time.Time
}

var _ StateStorage = (*Memory)(nil)

// NewMemory returns an empty Memory whose states expire ttl after their last Set.
func NewMemory(ttl time.Duration) *Memory {
	return &Memory{ttl: ttl, now: time.Now, states: map[string]entry{}}
}

// Get returns a copy of the state of key, or the zero State if it has none.
func (m *Memory) Get(_ context.Context, key string) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.states[key]
	if !ok {
		return State{}, nil
	}
	if m.expired(e, m.now()) {
		delete(m.states, key)
		return State{}, nil
	}
	return e.state.clone(), nil
}

// Set stores a copy of s as the state of key; setting the zero State deletes it.
func (m *Memory) Set(_ context.Context, key string, s State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.isZero() {
		delete(m.states, key)
		return nil
	}
	now := m.now()
	m.states[key] = entry{state: s.clone(), expires: now.Add(m.ttl)}
	// Sweeping once per len(states) writes keeps a write's average cost constant.
	if m.writes++; m.ttl > 0 && m.writes >= max(len(m.states), 64) {
		m.writes = 0
		for k, e := range m.states {
			if m.expired(e, now) {
				delete(m.states, k)
			}
		}
	}
	return nil
}

// Delete removes the state of key.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, key)
	return nil
}

func (m *Memory) expired(e entry, now time.Time) bool {
	return m.ttl > 0 && !now.Before(e.expires)
}
