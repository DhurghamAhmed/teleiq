package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// groupBot returns a bot whose handlers and middleware write to the returned log, and the errors
// that reach its ErrorHandler. The tests call dispatch directly, on one goroutine.
func groupBot(t *testing.T) (*Bot, *[]string, *[]error) {
	t.Helper()
	client, err := teleiq.NewClient(testToken)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	var errs []error
	b := New(client, WithErrorHandler(func(_ context.Context, _ *Context, err error) { errs = append(errs, err) }))
	return b, &log, &errs
}

func logged(log *[]string, name string) Handler {
	return func(context.Context, *Context) error { *log = append(*log, name); return nil }
}

func around(log *[]string, name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, c *Context) error {
			*log = append(*log, name+">")
			err := next(ctx, c)
			*log = append(*log, "<"+name)
			return err
		}
	}
}

func messageFrom(user int64, text string) *models.Update {
	return &models.Update{Message: &models.Message{Chat: models.Chat{ID: user, Type: "private"}, From: &models.User{ID: user}, Text: &text}}
}

var fromAdmin = FilterFunc(func(c *Context) bool { return c.Sender() != nil && c.Sender().ID == 1 })

func TestGroups(t *testing.T) {
	b, log, errs := groupBot(t)
	b.Use(around(log, "global"))
	admins := b.Group(fromAdmin)
	b.OnCommand("ban", logged(log, "not allowed"))
	admins.Use(around(log, "admins"))
	admins.OnCommand("ban", logged(log, "ban")) // added after the route above, but the group comes first
	shouts := admins.Group(FilterFunc(func(c *Context) bool { return strings.HasSuffix(c.Text(), "!") }))
	shouts.Use(around(log, "shouts"))
	shouts.OnMessage(logged(log, "shout"))
	b.OnCommand("start", logged(log, "start"))
	both := b.Group(fromAdmin, FilterFunc(func(c *Context) bool { return c.Text() == "both" }))
	both.OnMessage(logged(log, "both"))
	everyone := b.Group()
	everyone.Use(around(log, "everyone"))
	everyone.OnMessage(logged(log, "echo"))

	tests := []struct {
		name string
		u    *models.Update
		want string
	}{
		{"a command of the group", messageFrom(1, "/ban"), "global> admins> ban <admins <global"},
		{"the filter of the group does not match", messageFrom(2, "/ban"), "global> not allowed <global"},
		{"no route of the group matches", messageFrom(1, "/start"), "global> start <global"},
		{"a group inside a group", messageFrom(1, "hey!"), "global> admins> shouts> shout <shouts <admins <global"},
		{"every filter of a group matches", messageFrom(1, "both"), "global> both <global"},
		{"one filter of a group does not match", messageFrom(2, "both"), "global> everyone> echo <everyone <global"},
		{"a group without filters", messageFrom(2, "hey!"), "global> everyone> echo <everyone <global"},
		{"no route matches", &models.Update{InlineQuery: &models.InlineQuery{From: models.User{ID: 1}}}, "global> <global"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*log = nil
			b.dispatch(context.Background(), tt.u)
			if got := strings.Join(*log, " "); got != tt.want {
				t.Errorf("ran %q, want %q", got, tt.want)
			}
		})
	}
	if len(*errs) != 0 {
		t.Errorf("errors: %v", *errs)
	}
}

func TestGroupRoutes(t *testing.T) {
	b, log, _ := groupBot(t)
	g := b.Group()
	g.OnCommand("/start", logged(log, "command"))
	g.OnCallbackPrefix("color:", logged(log, "prefix"))
	g.OnCallback(logged(log, "callback"))
	g.OnInlineQuery(logged(log, "inline"))
	g.OnJoinRequest(logged(log, "join"))
	g.Handle(FilterFunc(func(c *Context) bool { return c.update.EditedMessage != nil }), logged(log, "edit"))
	g.OnMessage(logged(log, "message"))
	text := "x"
	for _, u := range []*models.Update{
		messageFrom(1, "/START now"),
		{CallbackQuery: &models.CallbackQuery{Data: teleiq.Ptr("color:red")}},
		{CallbackQuery: &models.CallbackQuery{Data: teleiq.Ptr("size")}},
		{InlineQuery: &models.InlineQuery{}},
		{ChatJoinRequest: &models.ChatJoinRequest{}},
		{EditedMessage: &models.Message{Text: &text}},
		messageFrom(1, "hi"),
	} {
		b.dispatch(context.Background(), u)
	}
	if got, want := strings.Join(*log, " "), "command prefix callback inline join edit message"; got != want {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestGroupHandled(t *testing.T) {
	b, _, _ := groupBot(t)
	var handled []bool
	b.Use(func(next Handler) Handler {
		return func(ctx context.Context, c *Context) error {
			err := next(ctx, c)
			handled = append(handled, c.handled)
			return err
		}
	})
	b.Group(fromAdmin).OnCommand("ban", func(context.Context, *Context) error { return nil })
	b.dispatch(context.Background(), messageFrom(1, "/ban"))
	b.dispatch(context.Background(), messageFrom(1, "/start"))
	if len(handled) != 2 || !handled[0] || handled[1] {
		t.Errorf("handled = %v, want [true false]: a group that no route of matches handles nothing", handled)
	}
}

func TestGroupErrors(t *testing.T) {
	b, _, errs := groupBot(t)
	boom := errors.New("boom")
	b.Group().OnMessage(func(context.Context, *Context) error { return boom })
	b.dispatch(context.Background(), messageFrom(1, "hi"))
	if len(*errs) != 1 || !errors.Is((*errs)[0], boom) {
		t.Errorf("errors = %v, want the error of the handler", *errs)
	}

	b, _, errs = groupBot(t)
	g := b.Group()
	g.Use(func(Handler) Handler { return nil })
	g.OnMessage(func(context.Context, *Context) error { return nil })
	b.dispatch(context.Background(), messageFrom(1, "hi"))
	if len(*errs) != 1 || !strings.Contains((*errs)[0].Error(), "nil handler") {
		t.Errorf("errors = %v, want a middleware that returned a nil handler", *errs)
	}
}

func TestGroupMisuse(t *testing.T) {
	noop := func(context.Context, *Context) error { return nil }
	tests := map[string]func(b *Bot){
		"nil filter":                 func(b *Bot) { b.Group(fromAdmin, nil) },
		"nil filter of an inner one": func(b *Bot) { b.Group().Group(nil) },
		"nil middleware":             func(b *Bot) { b.Group().Use(nil) },
		"nil filter of a route":      func(b *Bot) { b.Group().Handle(nil, noop) },
		"nil handler":                func(b *Bot) { b.Group().OnMessage(nil) },
		"invalid command":            func(b *Bot) { b.Group().OnCommand("say hi", noop) },
	}
	for name, misuse := range tests {
		t.Run(name, func(t *testing.T) {
			b, _, _ := groupBot(t)
			misuse(b)
			if err := b.Run(t.Context()); err == nil || !strings.HasPrefix(err.Error(), "bot: ") {
				t.Errorf("Run() = %v, want the misuse", err)
			}
		})
	}
}

// TestGroupAddWhileRunning adds handlers and middleware to a group while updates go through it; run
// with -race.
func TestGroupAddWhileRunning(t *testing.T) {
	b, _, _ := groupBot(t)
	g := b.Group()
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			g.OnMessage(func(context.Context, *Context) error { return nil })
			g.Use(func(h Handler) Handler { return h })
		}
	})
	wg.Go(func() {
		for range 1000 {
			b.dispatch(context.Background(), messageFrom(2, "hi"))
		}
	})
	wg.Wait()
}
