package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func TestOnInlineQuery(t *testing.T) {
	b, errs := newDispatchBot(t)
	var got []string
	b.OnInlineQuery(func(_ context.Context, c *Context) error {
		got = append(got, "query "+c.Update().InlineQuery.Query)
		return nil
	})
	b.Handle(FilterFunc(func(*Context) bool { return true }), func(_ context.Context, c *Context) error {
		got = append(got, updateKind(c.Update()))
		return nil
	})
	for _, u := range []*models.Update{
		{UpdateID: 1, InlineQuery: &models.InlineQuery{ID: "q", From: models.User{ID: 7}, Query: "cats"}},
		textUpdate(2, "hi"),
		{UpdateID: 3, ChosenInlineResult: &models.ChosenInlineResult{ResultID: "1", From: models.User{ID: 7}, Query: "cats"}},
		{UpdateID: 4, CallbackQuery: &models.CallbackQuery{ID: "c", From: models.User{ID: 7}, InlineMessageID: teleiq.Ptr("m")}},
		{UpdateID: 5, InlineQuery: &models.InlineQuery{ID: "q2", From: models.User{ID: 8}}},
	} {
		b.dispatch(context.Background(), u)
	}
	if got, want := strings.Join(got, ", "), "query cats, message, chosen_inline_result, callback_query, query "; got != want || len(*errs) != 0 {
		t.Errorf("handled %q with errors %v, want %q", got, *errs, want)
	}
}
