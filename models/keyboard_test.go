package models

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestKeyboardBuilders(t *testing.T) {
	tests := []struct {
		name     string
		got      any
		want     any
		wantJSON string
	}{
		{
			name:     "callback button",
			got:      NewCallbackButton("Red", "color:red"),
			want:     InlineKeyboardButton{Text: "Red", CallbackData: ptr("color:red")},
			wantJSON: `{"text":"Red","callback_data":"color:red"}`,
		},
		{
			name:     "callback button with empty data",
			got:      NewCallbackButton("", ""),
			want:     InlineKeyboardButton{CallbackData: ptr("")},
			wantJSON: `{"text":"","callback_data":""}`,
		},
		{
			name:     "url button",
			got:      NewURLButton("Docs", "https://core.telegram.org/bots/api"),
			want:     InlineKeyboardButton{Text: "Docs", URL: ptr("https://core.telegram.org/bots/api")},
			wantJSON: `{"text":"Docs","url":"https://core.telegram.org/bots/api"}`,
		},
		{
			name:     "row",
			got:      NewInlineRow(NewCallbackButton("A", "a"), NewURLButton("B", "tg://b")),
			want:     []InlineKeyboardButton{{Text: "A", CallbackData: ptr("a")}, {Text: "B", URL: ptr("tg://b")}},
			wantJSON: `[{"text":"A","callback_data":"a"},{"text":"B","url":"tg://b"}]`,
		},
		{
			name:     "empty row",
			got:      NewInlineRow(),
			want:     []InlineKeyboardButton{},
			wantJSON: `[]`,
		},
		{
			name: "keyboard",
			got: NewInlineKeyboard(
				NewInlineRow(NewCallbackButton("Red", "color:red"), NewCallbackButton("Green", "color:green")),
				NewInlineRow(NewURLButton("Help", "https://example.com/help")),
			),
			want: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: "Red", CallbackData: ptr("color:red")}, {Text: "Green", CallbackData: ptr("color:green")}},
				{{Text: "Help", URL: ptr("https://example.com/help")}},
			}},
			wantJSON: `{"inline_keyboard":[[{"text":"Red","callback_data":"color:red"},` +
				`{"text":"Green","callback_data":"color:green"}],[{"text":"Help","url":"https://example.com/help"}]]}`,
		},
		{
			name:     "empty keyboard",
			got:      NewInlineKeyboard(),
			want:     &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}},
			wantJSON: `{"inline_keyboard":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("got %#v, want %#v", tt.got, tt.want)
			}
			got, err := json.Marshal(tt.got)
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) || string(got) != tt.wantJSON {
				t.Errorf("JSON = %s, want %s (struct literal: %s)", got, tt.wantJSON, want)
			}
		})
	}
}

func TestButtonsDoNotShareData(t *testing.T) {
	a, b := NewCallbackButton("A", "a"), NewCallbackButton("B", "b")
	*a.CallbackData = "changed"
	if *b.CallbackData != "b" {
		t.Errorf("changing one button changed another: %q", *b.CallbackData)
	}
}

func ptr[T any](v T) *T { return &v }
