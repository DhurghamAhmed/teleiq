package bot_test

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestContextMe(t *testing.T) {
	srv, b := newBot(t)
	if me := b.NewContext(nil).Me(); me != nil {
		t.Fatalf("Me() = %+v before Run, want nil", me)
	}
	seen := make(chan models.User, 2)
	b.OnMessage(func(_ context.Context, c *bot.Context) error {
		me := c.Me()
		if me == nil {
			seen <- models.User{}
			return nil
		}
		seen <- *me
		me.FirstName = "Changed"
		return nil
	})
	stop := teleiqtest.Start(t, b.Run)
	srv.WaitHandled(t, srv.Push(teleiqtest.MessageUpdate(7, "hi")))
	srv.WaitHandled(t, srv.Push(teleiqtest.MessageUpdate(7, "hi")))
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		me := <-seen
		if me.ID != 123456 || !me.IsBot || me.FirstName != "Test Bot" || me.Username == nil || *me.Username != "test_bot" {
			t.Fatalf("Me() = %+v, want the bot that getMe describes, unchanged by another handler", me)
		}
	}
	if me := b.NewContext(nil).Me(); me.FirstName != "Test Bot" {
		t.Errorf("FirstName = %q after a handler changed its copy, want Test Bot", me.FirstName)
	}
}
