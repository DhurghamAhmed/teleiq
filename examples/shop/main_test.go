package main

import (
	"context"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestShop(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })
	send := func(u models.Update) { srv.WaitHandled(t, srv.Push(u)) }

	send(teleiqtest.MessageUpdate(7, "/start"))
	menu := srv.Requests("sendMessage")[0].Params
	if menu["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %v, want HTML from the defaults", menu["parse_mode"])
	}
	rows := menu["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	var layout []int
	for _, r := range rows {
		layout = append(layout, len(r.([]any)))
	}
	if len(layout) != 4 || layout[0] != 2 || layout[1] != 2 || layout[2] != 1 || layout[3] != 1 {
		t.Errorf("rows of %v buttons, want 2, 2, 1 and the close button", layout)
	}
	coffee := rows[0].([]any)[1].(map[string]any)["callback_data"].(string)
	if coffee != "buy:2:1" {
		t.Errorf("the data of Coffee = %q, want buy:2:1", coffee)
	}

	send(teleiqtest.CallbackUpdate(7, 1, coffee))
	send(teleiqtest.CallbackUpdate(7, 1, "buy:x"))
	send(teleiqtest.CallbackUpdate(7, 1, "close"))
	if edit := srv.Requests("editMessageText"); len(edit) != 1 || edit[0].Params["text"] != "You ordered <b>1</b> of product <code>2</code>." {
		t.Errorf("edits = %v", edit)
	}
	answers := srv.Requests("answerCallbackQuery")
	if len(answers) != 3 || answers[1].Params["text"] != "This button is out of date." {
		t.Errorf("answers = %v, want the order, the out-of-date button and the close", answers)
	}
	if len(srv.Requests("deleteMessage")) != 1 {
		t.Errorf("close did not delete the menu")
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
