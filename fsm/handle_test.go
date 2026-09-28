package fsm_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"testing"

	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/fsm"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// recorder is a Memory that logs each call, with its key, and can fail on any of them.
type recorder struct {
	*fsm.Memory
	log                    *[]string
	getErr, setErr, delErr error
}

func (r recorder) Get(ctx context.Context, key string) (fsm.State, error) {
	*r.log = append(*r.log, "get "+key)
	if r.getErr != nil {
		return fsm.State{}, r.getErr
	}
	return r.Memory.Get(ctx, key)
}

func (r recorder) Set(ctx context.Context, key string, s fsm.State) error {
	*r.log = append(*r.log, "set "+key)
	if r.setErr != nil {
		return r.setErr
	}
	return r.Memory.Set(ctx, key, s)
}

func (r recorder) Delete(ctx context.Context, key string) error {
	*r.log = append(*r.log, "delete "+key)
	if r.delErr != nil {
		return r.delErr
	}
	return r.Memory.Delete(ctx, key)
}

func TestHandle(t *testing.T) {
	ask := fsm.State{Name: "age", Data: map[string]string{"name": "Ann"}}
	errH, errGet, errSet, errDel := errors.New("h failed"), errors.New("get failed"), errors.New("set failed"), errors.New("delete failed")
	tests := []struct {
		name    string
		start   fsm.State
		h       func(s *fsm.State) error
		storage recorder
		wantLog []string
		wantErr error
		alsoErr error // a second error joined to wantErr
		want    fsm.State
	}{
		{
			name:    "new conversation",
			h:       func(s *fsm.State) error { *s = fsm.State{Name: "name"}; return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    fsm.State{Name: "name"},
		},
		{
			name:    "next step",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    ask,
		},
		{
			name:    "only the name changed",
			start:   ask,
			h:       func(s *fsm.State) error { s.Name = "confirm"; return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    fsm.State{Name: "confirm", Data: ask.Data},
		},
		{
			name:    "unchanged",
			start:   ask,
			h:       func(*fsm.State) error { return nil },
			wantLog: []string{"get -100:7", "h"},
			want:    ask,
		},
		{
			name:  "the same state set again",
			start: ask,
			h: func(s *fsm.State) error {
				*s = fsm.State{Name: "age", Data: map[string]string{"name": "Ann"}}
				return nil
			},
			wantLog: []string{"get -100:7", "h"},
			want:    ask,
		},
		{
			name:    "value added in place",
			start:   ask,
			h:       func(s *fsm.State) error { s.Data["age"] = "30"; return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    fsm.State{Name: "age", Data: map[string]string{"name": "Ann", "age": "30"}},
		},
		{
			name:    "value changed in place",
			start:   ask,
			h:       func(s *fsm.State) error { s.Data["name"] = "Bob"; return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    fsm.State{Name: "age", Data: map[string]string{"name": "Bob"}},
		},
		{
			name:    "value deleted in place",
			start:   ask,
			h:       func(s *fsm.State) error { delete(s.Data, "name"); return nil },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			want:    fsm.State{Name: "age"},
		},
		{
			name:    "zero state ends the conversation",
			start:   ask,
			h:       func(s *fsm.State) error { *s = fsm.State{}; return nil },
			wantLog: []string{"get -100:7", "h", "delete -100:7"},
		},
		{
			name:  "no name and emptied data end the conversation",
			start: ask,
			h: func(s *fsm.State) error {
				s.Name = ""
				clear(s.Data)
				return nil
			},
			wantLog: []string{"get -100:7", "h", "delete -100:7"},
		},
		{
			name:    "zero state without a conversation",
			h:       func(s *fsm.State) error { *s = fsm.State{}; return nil },
			wantLog: []string{"get -100:7", "h"},
		},
		{
			name:    "h fails",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return errH },
			wantLog: []string{"get -100:7", "h"},
			wantErr: errH,
			want:    fsm.State{Name: "name"},
		},
		{
			name:    "h fails after an edit in place",
			start:   ask,
			h:       func(s *fsm.State) error { s.Data["name"] = "Bob"; return errH },
			wantLog: []string{"get -100:7", "h"},
			wantErr: errH,
			want:    ask,
		},
		{
			name:    "h fails with StoreAnyway",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return fsm.StoreAnyway(errH) },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			wantErr: errH,
			want:    ask,
		},
		{
			name:    "h ends the conversation and fails with StoreAnyway",
			start:   ask,
			h:       func(s *fsm.State) error { *s = fsm.State{}; return fsm.StoreAnyway(errH) },
			wantLog: []string{"get -100:7", "h", "delete -100:7"},
			wantErr: errH,
		},
		{
			name:    "h fails with StoreAnyway after an edit in place",
			start:   ask,
			h:       func(s *fsm.State) error { s.Data["name"] = "Bob"; return fsm.StoreAnyway(errH) },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			wantErr: errH,
			want:    fsm.State{Name: "age", Data: map[string]string{"name": "Bob"}},
		},
		{
			name:    "h fails with StoreAnyway, unchanged",
			start:   ask,
			h:       func(*fsm.State) error { return fsm.StoreAnyway(errH) },
			wantLog: []string{"get -100:7", "h"},
			wantErr: errH,
			want:    ask,
		},
		{
			name:    "h fails with a wrapped StoreAnyway",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return fmt.Errorf("badge: %w", fsm.StoreAnyway(errH)) },
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			wantErr: errH,
			want:    ask,
		},
		{
			name:    "h fails with StoreAnyway and set fails",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return fsm.StoreAnyway(errH) },
			storage: recorder{setErr: errSet},
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			wantErr: errH,
			alsoErr: errSet,
			want:    fsm.State{Name: "name"},
		},
		{
			name:    "h ends the conversation and fails with StoreAnyway, and delete fails",
			start:   ask,
			h:       func(s *fsm.State) error { *s = fsm.State{}; return fsm.StoreAnyway(errH) },
			storage: recorder{delErr: errDel},
			wantLog: []string{"get -100:7", "h", "delete -100:7"},
			wantErr: errH,
			alsoErr: errDel,
			want:    ask,
		},
		{
			name:    "get fails",
			start:   ask,
			h:       func(*fsm.State) error { return nil },
			storage: recorder{getErr: errGet},
			wantLog: []string{"get -100:7"},
			wantErr: errGet,
			want:    ask,
		},
		{
			name:    "set fails",
			start:   fsm.State{Name: "name"},
			h:       func(s *fsm.State) error { *s = ask; return nil },
			storage: recorder{setErr: errSet},
			wantLog: []string{"get -100:7", "h", "set -100:7"},
			wantErr: errSet,
			want:    fsm.State{Name: "name"},
		},
		{
			name:    "delete fails",
			start:   ask,
			h:       func(s *fsm.State) error { *s = fsm.State{}; return nil },
			storage: recorder{delErr: errDel},
			wantLog: []string{"get -100:7", "h", "delete -100:7"},
			wantErr: errDel,
			want:    ask,
		},
	}
	b := newBot(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var log []string
			storage := tt.storage
			storage.Memory, storage.log = fsm.NewMemory(0), &log
			if err := storage.Memory.Set(ctx, "-100:7", tt.start); err != nil {
				t.Fatal(err)
			}
			handler := fsm.Handle(storage, func(_ context.Context, _ *bot.Context, s *fsm.State) error {
				log = append(log, "h")
				return tt.h(s)
			})

			u := teleiqtest.GroupMessageUpdate(-100, 7, "Ann")
			err := handler(ctx, b.NewContext(&u))
			if !errors.Is(err, tt.wantErr) || tt.alsoErr != nil && !errors.Is(err, tt.alsoErr) {
				t.Errorf("handler returned %v, want %v and %v", err, tt.wantErr, tt.alsoErr)
			}
			// A single error comes back as it is, not joined to a nil one.
			if _, joined := err.(interface{ Unwrap() []error }); joined != (tt.alsoErr != nil) {
				t.Errorf("handler returned %#v, joined = %v, want %v", err, joined, !joined)
			}
			if !reflect.DeepEqual(log, tt.wantLog) {
				t.Errorf("calls %q, want %q", log, tt.wantLog)
			}
			if got, _ := storage.Memory.Get(ctx, "-100:7"); !equalStates(got, tt.want) {
				t.Errorf("stored %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStoreAnyway(t *testing.T) {
	errH := errors.New("h failed")
	if err := fsm.StoreAnyway(nil); err != nil {
		t.Errorf("StoreAnyway(nil) = %v, want nil", err)
	}
	err := fsm.StoreAnyway(errH)
	if errors.Unwrap(err) != errH || err.Error() != errH.Error() {
		t.Errorf("StoreAnyway(%v) = %v, want the same text, unwrapping to it", errH, err)
	}
}

func equalStates(a, b fsm.State) bool {
	return a.Name == b.Name && maps.Equal(a.Data, b.Data)
}

func TestHandleKeys(t *testing.T) {
	ctx := context.Background()
	b := newBot(t)
	store := fsm.NewMemory(0)
	count := fsm.Handle(store, func(_ context.Context, _ *bot.Context, s *fsm.State) error {
		n, _ := strconv.Atoi(s.Data["n"])
		*s = fsm.State{Name: "counting", Data: map[string]string{"n": strconv.Itoa(n + 1)}}
		return nil
	})
	for _, u := range []struct{ chat, user int64 }{{-100, 7}, {-100, 8}, {-100, 7}, {-200, 7}, {7, 7}, {-100, 7}} {
		update := teleiqtest.GroupMessageUpdate(u.chat, u.user, "+1")
		if err := count(ctx, b.NewContext(&update)); err != nil {
			t.Fatal(err)
		}
	}
	for key, want := range map[string]string{"-100:7": "3", "-100:8": "1", "-200:7": "1", "7:7": "1", "-100:9": ""} {
		t.Run(key, func(t *testing.T) {
			if s, _ := store.Get(ctx, key); s.Data["n"] != want {
				t.Errorf("count of %s = %q, want %q", key, s.Data["n"], want)
			}
		})
	}
}

func TestHandleNil(t *testing.T) {
	b := newBot(t)
	u := teleiqtest.MessageUpdate(7, "hi")
	h := func(context.Context, *bot.Context, *fsm.State) error { return nil }
	tests := []struct {
		name    string
		handler bot.Handler
	}{
		{"nil storage", fsm.Handle(nil, h)},
		{"nil handler", fsm.Handle(fsm.NewMemory(0), nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.handler(context.Background(), b.NewContext(&u)); err == nil {
				t.Error("handler returned nil, want an error")
			}
		})
	}
}
