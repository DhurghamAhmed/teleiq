package fsm_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/fsm"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestConversationSteps(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	store := fsm.NewMemory(time.Hour)
	b := bot.New(client)
	register := fsm.NewConversation(b, store)
	b.OnCommand("cancel", register.End(bot.ReplyWith("Cancelled.")))
	b.OnCommand("register", register.Begin("name", bot.ReplyWith("Name?")))
	register.Step("name", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		s.Name = "age"
		s.Set("name", c.Text())
		return c.Send(ctx, "Age?")
	})
	register.Step("age", func(ctx context.Context, c *bot.Context, s *fsm.State) error {
		if _, err := strconv.Atoi(c.Text()); err != nil {
			return c.Send(ctx, "A number, please.")
		}
		name := s.Data["name"]
		*s = fsm.State{}
		return c.Send(ctx, "Registered "+name+", "+c.Text()+".")
	})
	b.OnCommand("help", bot.ReplyWith("Help."))
	b.OnMessage(bot.ReplyWith("No conversation."))

	stop := teleiqtest.Start(t, b.Run)
	script := []struct {
		user int64
		text string
		want string
	}{
		{7, "hi", "No conversation."},
		{7, "/register", "Name?"},
		{7, "/help", "Help."},          // a command is not an answer
		{8, "Ann", "No conversation."}, // another user has no conversation
		{7, "Ann", "Age?"},
		{7, "/register", "Name?"}, // starting again drops the name
		{7, "Bob", "Age?"},
		{7, "old", "A number, please."}, // the step stays
		{7, "30", "Registered Bob, 30."},
		{7, "31", "No conversation."}, // the conversation ended
		{7, "/register", "Name?"},
		{7, "/cancel", "Cancelled."},
		{7, "Ann", "No conversation."},
		{7, "/cancel", "Cancelled."}, // without a conversation
	}
	for _, s := range script {
		srv.WaitHandled(t, srv.Push(teleiqtest.MessageUpdate(s.user, s.text)))
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	sent := srv.Requests("sendMessage")
	if len(sent) != len(script) {
		t.Fatalf("%d replies, want %d", len(sent), len(script))
	}
	for i, s := range script {
		if got := sent[i].Params["text"]; got != s.want {
			t.Errorf("%d %q: replied %q, want %q", s.user, s.text, got, s.want)
		}
	}
	if st, err := store.Get(context.Background(), "7:7"); err != nil || st.Name != "" || st.Data != nil {
		t.Errorf("state after the conversation = %+v, %v; want none", st, err)
	}
}

func TestConversationErrors(t *testing.T) {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		t.Fatal(err)
	}
	b := bot.New(client)
	store := fsm.NewMemory(time.Hour)
	cv := fsm.NewConversation(b.Group(), store) // a group works as the bot does
	u := teleiqtest.MessageUpdate(7, "/register")
	ctx := context.Background()
	failed := errors.New("reply failed")

	if err := cv.Begin("name", func(context.Context, *bot.Context) error { return failed })(ctx, b.NewContext(&u)); !errors.Is(err, failed) {
		t.Errorf("Begin() = %v, want the error of its handler", err)
	}
	if st, _ := store.Get(ctx, "7:7"); st.Name != "" {
		t.Errorf("a Begin whose handler failed stored %+v", st)
	}
	ok := func(context.Context, *bot.Context) error { return nil }
	for name, h := range map[string]bot.Handler{
		"empty step":        cv.Begin("", ok),
		"nil Begin handler": cv.Begin("name", nil),
		"nil End handler":   cv.End(nil),
	} {
		if err := h(ctx, b.NewContext(&u)); err == nil || !strings.HasPrefix(err.Error(), "fsm: ") {
			t.Errorf("%s: %v, want an error", name, err)
		}
	}

	if err := store.Set(ctx, "7:7", fsm.State{Name: "age", Data: map[string]string{"name": "Ann"}}); err != nil {
		t.Fatal(err)
	}
	if err := cv.Begin("name", ok)(ctx, b.NewContext(&u)); err != nil {
		t.Fatal(err)
	}
	if st, _ := store.Get(ctx, "7:7"); st.Name != "name" || len(st.Data) != 0 {
		t.Errorf("Begin in the middle of a conversation left %+v, want the step name without data", st)
	}
	if err := cv.End(func(context.Context, *bot.Context) error { return failed })(ctx, b.NewContext(&u)); !errors.Is(err, failed) {
		t.Errorf("End() = %v, want the error of its handler", err)
	}
	if st, _ := store.Get(ctx, "7:7"); st.Name != "name" {
		t.Errorf("an End whose handler failed ended the conversation: %+v", st)
	}
}

func TestStateSet(t *testing.T) {
	var s fsm.State
	s.Set("a", "1")
	s.Set("b", "2")
	s.Set("a", "3")
	if len(s.Data) != 2 || s.Data["a"] != "3" || s.Data["b"] != "2" {
		t.Errorf("Data = %v, want a:3 and b:2", s.Data)
	}
}
