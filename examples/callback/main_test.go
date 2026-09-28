package main

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestCallback(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error { return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL)) })

	srv.Push(teleiqtest.MessageUpdate(7, "/menu"))
	menu := srv.Wait(t, "sendMessage", 1)[0].Params
	keyboard := fmt.Sprint(menu["reply_markup"])
	if menu["text"] != "Pick a color:" || keyboard != "map[inline_keyboard:[[map[callback_data:color:red text:Red] map[callback_data:color:green text:Green] map[callback_data:color:blue text:Blue]]]]" {
		t.Errorf("menu = %v", menu)
	}

	id := srv.Push(teleiqtest.CallbackUpdate(7, 42, "color:green"))
	if p := srv.Wait(t, "answerCallbackQuery", 1)[0].Params; p["callback_query_id"] != strconv.FormatInt(id, 10) || p["text"] != "You picked green" {
		t.Errorf("answer = %v", p)
	}
	if p := srv.Wait(t, "editMessageText", 1)[0].Params; p["chat_id"] != 7.0 || p["message_id"] != 42.0 || p["text"] != "Your color: green" {
		t.Errorf("edit = %v", p)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
