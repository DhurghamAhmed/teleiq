package teleiqtest_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func ids(updates []models.Update) []int64 {
	var out []int64
	for _, u := range updates {
		out = append(out, u.UpdateID)
	}
	return out
}

func TestGetUpdates(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	c := newClient(t, srv)
	ctx := context.Background()
	for _, text := range []string{"a", "b", "c"} {
		srv.Push(teleiqtest.MessageUpdate(7, text))
	}

	if got, err := c.GetUpdates(ctx, teleiq.GetUpdatesParams{Limit: teleiq.Ptr(2)}); err != nil || !slices.Equal(ids(got), []int64{1, 2}) {
		t.Errorf("GetUpdates(limit 2) = %v, %v; want [1 2]", ids(got), err)
	}
	if got, err := c.GetUpdates(ctx, teleiq.GetUpdatesParams{Offset: teleiq.Ptr(int64(3))}); err != nil || !slices.Equal(ids(got), []int64{3}) {
		t.Errorf("GetUpdates(offset 3) = %v, %v; want [3]", ids(got), err)
	}
	if got, err := c.GetUpdates(ctx, teleiq.GetUpdatesParams{}); err != nil || !slices.Equal(ids(got), []int64{3}) {
		t.Errorf("GetUpdates() = %v, %v; want the updates below offset 3 forgotten", ids(got), err)
	}

	// A long poll waits for the next update.
	go func() {
		time.Sleep(50 * time.Millisecond)
		srv.Push(teleiqtest.MessageUpdate(7, "d"))
	}()
	start := time.Now()
	got, err := c.GetUpdates(ctx, teleiq.GetUpdatesParams{Offset: teleiq.Ptr(int64(4)), Timeout: teleiq.Ptr(10)})
	if err != nil || !slices.Equal(ids(got), []int64{4}) || time.Since(start) > 5*time.Second {
		t.Errorf("long poll = %v, %v after %v; want update 4 as soon as it was pushed", ids(got), err, time.Since(start))
	}
	if n := len(srv.Requests("")); n != 0 {
		t.Errorf("%d requests recorded, want getUpdates left out", n)
	}
}

func TestCloseEndsLongPolls(t *testing.T) {
	srv := teleiqtest.NewServer()
	c := newClient(t, srv)
	done := make(chan error, 1)
	go func() {
		_, err := c.GetUpdates(context.Background(), teleiq.GetUpdatesParams{Timeout: teleiq.Ptr(30)})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	srv.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a long poll kept Close waiting")
	}
}

// TestBot runs a bot of package bot against the server, as the tests of a bot would.
func TestBot(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	var handled atomic.Int64
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		c, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
		if err != nil {
			return err
		}
		b := bot.New(c)
		b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
			_, err := c.Reply(ctx, "Hello, "+c.Sender().FirstName+"!")
			return err
		})
		b.OnCallback(func(ctx context.Context, c *bot.Context) error {
			handled.Add(1)
			return c.Client().AnswerCallbackQuery(ctx, teleiq.AnswerCallbackQueryParams{CallbackQueryID: c.Update().CallbackQuery.ID})
		})
		b.OnMessage(func(context.Context, *bot.Context) error {
			handled.Add(1)
			return nil
		})
		return b.Run(ctx)
	})

	srv.Push(teleiqtest.MessageUpdate(7, "/start"))
	if got := srv.Wait(t, "sendMessage", 1)[0].Params; got["chat_id"] != 7.0 || got["text"] != "Hello, User7!" {
		t.Errorf("reply = %v", got)
	}
	id := srv.Push(teleiqtest.MessageUpdate(7, "no reply to this"))
	srv.WaitHandled(t, id)
	if handled.Load() != 1 {
		t.Errorf("after WaitHandled, %d updates handled, want 1", handled.Load())
	}
	srv.Push(teleiqtest.CallbackUpdate(7, 42, "color:red"))
	if q := srv.Wait(t, "answerCallbackQuery", 1)[0].Params; q["callback_query_id"] == "" {
		t.Errorf("answerCallbackQuery = %v", q)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Errorf("a second stop() = %v, want the same nil", err)
	}
}

func TestStartReturnsTheError(t *testing.T) {
	failed := errors.New("failed")
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		<-ctx.Done()
		return failed
	})
	if err := stop(); !errors.Is(err, failed) {
		t.Errorf("stop() = %v, want the error of run", err)
	}
}

func TestBuilders(t *testing.T) {
	tests := []struct {
		text   string
		length int // of the bot_command entity; 0 for none
	}{
		{"/start", 6},
		{"/start@test_bot hello", 15},
		{"/ابدأ now", 5},
		{"hello", 0},
		{"/", 0},
	}
	for _, tt := range tests {
		m := teleiqtest.MessageUpdate(7, tt.text).Message
		if got := len(m.Entities); (got == 1) != (tt.length > 0) || got == 1 && (m.Entities[0].Type != "bot_command" || m.Entities[0].Length != tt.length) {
			t.Errorf("MessageUpdate(%q).Entities = %+v, want a command of length %d", tt.text, m.Entities, tt.length)
		}
		if m.Chat.ID != 7 || m.Chat.Type != "private" || m.From.ID != 7 || m.From.FirstName != "User7" || m.From.LastName != nil || m.From.Username != nil {
			t.Errorf("MessageUpdate(7, %q) = %+v", tt.text, m)
		}
	}
	for chat, want := range map[int64]string{-1001234567890: "supergroup Test supergroup", -42: "group Test group", 7: "private <nil>"} {
		c := teleiqtest.GroupMessageUpdate(chat, 7, "hi").Message.Chat
		title := "<nil>"
		if c.Title != nil {
			title = *c.Title
		}
		if got := c.Type + " " + title; got != want {
			t.Errorf("GroupMessageUpdate(%d).Chat = %q, want %q", chat, got, want)
		}
	}
	q := teleiqtest.CallbackUpdate(7, 42, "color:red").CallbackQuery
	if m, ok := q.Message.(*models.Message); !ok || m.MessageID != 42 || m.Date == 0 || *q.Data != "color:red" || q.From.FirstName != "User7" {
		t.Errorf("CallbackUpdate() = %+v, want the press under message 42", q)
	}
}

func TestInlineQueryUpdate(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	u := teleiqtest.InlineQueryUpdate(7, "cats")
	if q := u.InlineQuery; q == nil || q.ID != "" || q.From.ID != 7 || q.From.FirstName != "User7" || q.Query != "cats" || q.ChatType == nil || *q.ChatType != "sender" {
		t.Fatalf("InlineQueryUpdate(7, cats) = %+v", u.InlineQuery)
	}
	id := srv.Push(u)
	given := teleiqtest.InlineQueryUpdate(7, "dogs")
	given.InlineQuery.ID = "mine"
	srv.Push(given)
	got, err := newClient(t, srv).GetUpdates(context.Background(), teleiq.GetUpdatesParams{})
	if err != nil || len(got) != 2 || got[0].InlineQuery.ID != strconv.FormatInt(id, 10) || got[1].InlineQuery.ID != "mine" {
		t.Errorf("GetUpdates = %v, %v; want the queries with the IDs %d and mine", got, err, id)
	}
}
