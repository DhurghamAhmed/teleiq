package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// layout writes the texts of the buttons of a keyboard, rows apart with "|".
func layout(m *InlineKeyboardMarkup) string {
	var rows []string
	for _, r := range m.InlineKeyboard {
		var texts []string
		for _, b := range r {
			texts = append(texts, b.Text)
		}
		rows = append(rows, strings.Join(texts, " "))
	}
	return strings.Join(rows, " | ")
}

func buttons(texts ...string) []InlineKeyboardButton {
	var bs []InlineKeyboardButton
	for _, t := range texts {
		bs = append(bs, NewCallbackButton(t, t))
	}
	return bs
}

func TestInlineGrid(t *testing.T) {
	tests := []struct {
		name  string
		build func() *InlineGrid
		want  string
	}{
		{"rows of two", func() *InlineGrid { return NewInlineGrid(2).Add(buttons("1", "2", "3", "4", "5")...) }, "1 2 | 3 4 | 5"},
		{"a full row", func() *InlineGrid { return NewInlineGrid(3).Add(buttons("1", "2", "3")...) }, "1 2 3"},
		{"no columns", func() *InlineGrid { return NewInlineGrid(0).Add(buttons("1", "2", "3", "4")...) }, "1 2 3 4"},
		{"negative columns", func() *InlineGrid { return NewInlineGrid(-1).Add(buttons("1", "2")...) }, "1 2"},
		{"one by one", func() *InlineGrid {
			g := NewInlineGrid(2)
			for _, b := range buttons("1", "2", "3") {
				g.Add(b)
			}
			return g
		}, "1 2 | 3"},
		{"a row between", func() *InlineGrid {
			return NewInlineGrid(2).Add(buttons("1", "2", "3")...).Row(buttons("X")...).Add(buttons("4", "5")...)
		}, "1 2 | 3 | X | 4 5"},
		{"a row first", func() *InlineGrid { return NewInlineGrid(2).Row(buttons("X")...).Add(buttons("1", "2", "3")...) }, "X | 1 2 | 3"},
		{"a row wider than the columns", func() *InlineGrid { return NewInlineGrid(2).Row(buttons("A", "B", "C")...) }, "A B C"},
		{"an empty row ends the row", func() *InlineGrid { return NewInlineGrid(3).Add(buttons("1")...).Row().Add(buttons("2")...) }, "1 | 2"},
		{"an empty row at the start", func() *InlineGrid { return NewInlineGrid(3).Row().Add(buttons("1")...) }, "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layout(tt.build().Markup()); got != tt.want {
				t.Errorf("layout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInlineGridMarkup(t *testing.T) {
	got, err := json.Marshal(NewInlineGrid(2).Markup())
	if err != nil || string(got) != `{"inline_keyboard":[]}` {
		t.Errorf("an empty grid = %s, %v; want an empty keyboard", got, err)
	}

	g := NewInlineGrid(3).Add(buttons("1", "2")...)
	m := g.Markup()
	g.Add(buttons("3")...).Row(buttons("X")...)
	m.InlineKeyboard[0][0].Text = "changed"
	if layout(m) != "changed 2" || layout(g.Markup()) != "1 2 3 | X" {
		t.Errorf("Markup() = %q and then %q; want keyboards independent of the grid", layout(m), layout(g.Markup()))
	}
}
