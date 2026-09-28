package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestInline(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		return run(ctx, teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	})

	tests := []struct{ query, want string }{
		{" a<b ", "map[cache_time:3600 inline_query_id:1 results:[" +
			"map[description:a<b id:bold input_message_content:map[message_text:<b>a&lt;b</b> parse_mode:HTML] title:Bold type:article] " +
			"map[description:a<b id:italic input_message_content:map[message_text:<i>a&lt;b</i> parse_mode:HTML] title:Italic type:article] " +
			"map[description:a<b id:code input_message_content:map[message_text:<code>a&lt;b</code> parse_mode:HTML] title:Code type:article]]]"},
		{"  ", "map[button:map[start_parameter:help text:How to use] inline_query_id:2 results:[]]"},
	}
	for i, tt := range tests {
		srv.Push(teleiqtest.InlineQueryUpdate(7, tt.query))
		if got := fmt.Sprint(srv.Wait(t, "answerInlineQuery", i+1)[i].Params); got != tt.want {
			t.Errorf("the answer to %q = %s\nwant %s", tt.query, got, tt.want)
		}
	}

	srv.Push(teleiqtest.MessageUpdate(7, "/start help"))
	if got := srv.Wait(t, "sendMessage", 1)[0].Params; got["chat_id"] != 7.0 || got["text"] != "Type my username and a text in any chat, then pick a style." {
		t.Errorf("/start help sent %v", got)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}
