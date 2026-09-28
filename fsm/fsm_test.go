package fsm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/fsm"
	"github.com/DhurghamAhmed/teleiq/models"
)

func newBot(t *testing.T) *bot.Bot {
	t.Helper()
	client, err := teleiq.NewClient("123:test")
	if err != nil {
		t.Fatal(err)
	}
	return bot.New(client)
}

func contextFor(t *testing.T, b *bot.Bot, update string) *bot.Context {
	t.Helper()
	var u models.Update
	if err := json.Unmarshal([]byte(update), &u); err != nil {
		t.Fatal(err)
	}
	return b.NewContext(&u)
}

func text(chat, user int64, s string) string {
	b, _ := json.Marshal(s)
	return `{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":` + itoa(chat) + `,"type":"group"},` +
		`"from":{"id":` + itoa(user) + `,"is_bot":false,"first_name":"U"},"text":` + string(b) + `}}`
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestKey(t *testing.T) {
	b := newBot(t)
	tests := []struct{ name, update, want string }{
		{"user in a group", text(-100, 7, "hi"), "-100:7"},
		{"inline query without a chat", `{"update_id":1,"inline_query":{"id":"i","from":{"id":7,"is_bot":false,"first_name":"U"},"query":"","offset":""}}`, "0:7"},
		{"channel post without a sender", `{"update_id":1,"channel_post":{"message_id":1,"date":1,"chat":{"id":-300,"type":"channel"}}}`, "-300:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fsm.Key(contextFor(t, b, tt.update)); got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

type failingStorage struct{ fsm.StateStorage }

func (failingStorage) Get(context.Context, string) (fsm.State, error) {
	return fsm.State{Name: "ask_name"}, errors.New("storage is down")
}

func TestInState(t *testing.T) {
	ctx := context.Background()
	b := newBot(t)
	store := fsm.NewMemory(0)
	_ = store.Set(ctx, "-100:7", fsm.State{Name: "ask_name"})
	tests := []struct {
		name    string
		storage fsm.StateStorage
		states  []string
		update  string
		want    bool
	}{
		{"user at the step", store, []string{"ask_name"}, text(-100, 7, "Ann"), true},
		{"one of several steps", store, []string{"ask_age", "ask_name"}, text(-100, 7, "Ann"), true},
		{"another step", store, []string{"ask_age"}, text(-100, 7, "Ann"), false},
		{"another user in the chat", store, []string{"ask_name"}, text(-100, 8, "Ann"), false},
		{"the same user in another chat", store, []string{"ask_name"}, text(-200, 7, "Ann"), false},
		{"failing storage", failingStorage{}, []string{"ask_name"}, text(-100, 7, "Ann"), false},
		{"nil storage", nil, []string{"ask_name"}, text(-100, 7, "Ann"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fsm.InState(tt.storage, tt.states...).Match(contextFor(t, b, tt.update)); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConversation runs the handlers of a two-step registration the way a bot would route them.
func TestConversation(t *testing.T) {
	ctx := context.Background()
	b := newBot(t)
	store := fsm.NewMemory(0)
	var registered []string
	steps := []struct {
		filter  bot.Filter
		handler func(c *bot.Context) error
	}{
		{fsm.InState(store, "ask_name"), func(c *bot.Context) error {
			return store.Set(ctx, fsm.Key(c), fsm.State{Name: "ask_age", Data: map[string]string{"name": *c.Message().Text}})
		}},
		{fsm.InState(store, "ask_age"), func(c *bot.Context) error {
			s, err := store.Get(ctx, fsm.Key(c))
			if err != nil {
				return err
			}
			registered = append(registered, s.Data["name"]+" is "+*c.Message().Text)
			return store.Delete(ctx, fsm.Key(c))
		}},
		{bot.FilterFunc(func(c *bot.Context) bool { return c.Command() == "register" }), func(c *bot.Context) error {
			return store.Set(ctx, fsm.Key(c), fsm.State{Name: "ask_name"})
		}},
	}
	handle := func(update string) {
		c := contextFor(t, b, update)
		for _, s := range steps {
			if s.filter.Match(c) {
				if err := s.handler(c); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}

	handle(text(-100, 7, "/register"))
	handle(text(-100, 8, "Bob")) // another user of the group is not in the conversation
	handle(text(-100, 7, "Ann"))
	handle(text(-100, 7, "30"))
	handle(text(-100, 7, "31")) // the conversation is over

	if len(registered) != 1 || registered[0] != "Ann is 30" {
		t.Errorf("registered %q, want [Ann is 30]", registered)
	}
	if s, _ := store.Get(ctx, "-100:7"); s.Name != "" {
		t.Errorf("state after the conversation = %+v, want none", s)
	}
}
