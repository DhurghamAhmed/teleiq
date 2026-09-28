package fsm

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestMemory(ttl time.Duration) (*Memory, *clock) {
	c := &clock{t: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	m := NewMemory(ttl)
	m.now = c.now
	return m, c
}

func TestMemory(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		steps func(m *Memory, c *clock)
		want  State
	}{
		{name: "missing key", steps: func(*Memory, *clock) {}, want: State{}},
		{
			name: "set",
			steps: func(m *Memory, _ *clock) {
				_ = m.Set(ctx, "k", State{Name: "ask_age", Data: map[string]string{"name": "Ann"}})
			},
			want: State{Name: "ask_age", Data: map[string]string{"name": "Ann"}},
		},
		{
			name: "set again",
			steps: func(m *Memory, _ *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				_ = m.Set(ctx, "k", State{Name: "b"})
			},
			want: State{Name: "b"},
		},
		{
			name: "delete",
			steps: func(m *Memory, _ *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				_ = m.Delete(ctx, "k")
			},
			want: State{},
		},
		{
			name: "setting the zero state deletes",
			steps: func(m *Memory, _ *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				_ = m.Set(ctx, "k", State{})
			},
			want: State{},
		},
		{
			name: "before it expires",
			steps: func(m *Memory, c *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				c.t = c.t.Add(59 * time.Minute)
			},
			want: State{Name: "a"},
		},
		{
			name: "expired",
			steps: func(m *Memory, c *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				c.t = c.t.Add(time.Hour)
			},
			want: State{},
		},
		{
			name: "a new set restarts the time",
			steps: func(m *Memory, c *clock) {
				_ = m.Set(ctx, "k", State{Name: "a"})
				c.t = c.t.Add(50 * time.Minute)
				_ = m.Set(ctx, "k", State{Name: "b"})
				c.t = c.t.Add(50 * time.Minute)
			},
			want: State{Name: "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, c := newTestMemory(time.Hour)
			tt.steps(m, c)
			got, err := m.Get(ctx, "k")
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestMemoryCopies(t *testing.T) {
	ctx := context.Background()
	m := NewMemory(0)
	data := map[string]string{"name": "Ann"}
	_ = m.Set(ctx, "k", State{Name: "a", Data: data})
	data["name"] = "changed after Set"
	got, _ := m.Get(ctx, "k")
	got.Data["name"] = "changed after Get"
	if again, _ := m.Get(ctx, "k"); again.Data["name"] != "Ann" {
		t.Errorf("stored data = %v, want it unaffected by the callers", again.Data)
	}
}

func TestMemoryForgetsExpiredStates(t *testing.T) {
	ctx := context.Background()
	m, c := newTestMemory(time.Hour)
	for i := range 1000 {
		_ = m.Set(ctx, fmt.Sprint("abandoned", i), State{Name: "a"})
	}
	c.t = c.t.Add(2 * time.Hour)
	for i := range 1000 {
		_ = m.Set(ctx, fmt.Sprint("active", i%10), State{Name: "a"})
	}
	if n := len(m.states); n > 64 {
		t.Errorf("%d states kept, want the abandoned ones swept away", n)
	}

	forever := NewMemory(0)
	_ = forever.Set(ctx, "k", State{Name: "a"})
	if got, _ := forever.Get(ctx, "k"); got.Name != "a" {
		t.Error("a Memory without a ttl lost a state")
	}
}

func TestMemoryConcurrentUse(t *testing.T) {
	ctx := context.Background()
	m := NewMemory(time.Hour)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				key := fmt.Sprint(g, ":", i%5)
				_ = m.Set(ctx, key, State{Name: "a", Data: map[string]string{"i": fmt.Sprint(i)}})
				s, _ := m.Get(ctx, key)
				s.Data["i"] = "x"
				if i%7 == 0 {
					_ = m.Delete(ctx, key)
				}
			}
		})
	}
	wg.Wait()
}
