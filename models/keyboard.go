package models

import "slices"

// NewInlineKeyboard returns an inline keyboard with the given rows of buttons.
func NewInlineKeyboard(rows ...[]InlineKeyboardButton) *InlineKeyboardMarkup {
	if rows == nil {
		rows = [][]InlineKeyboardButton{}
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// NewInlineRow returns a row of inline buttons for NewInlineKeyboard.
func NewInlineRow(buttons ...InlineKeyboardButton) []InlineKeyboardButton {
	if buttons == nil {
		buttons = []InlineKeyboardButton{}
	}
	return buttons
}

// NewCallbackButton returns an inline button that sends data in a callback query.
func NewCallbackButton(text, data string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, CallbackData: &data}
}

// NewURLButton returns an inline button that opens url when it is pressed.
func NewURLButton(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, URL: &url}
}

// InlineGrid builds an inline keyboard from buttons added one by one into rows.
type InlineGrid struct {
	columns int
	rows    [][]InlineKeyboardButton
	open    bool // the last row takes the next buttons that Add adds
}

// NewInlineGrid returns an InlineGrid with rows of up to columns buttons.
func NewInlineGrid(columns int) *InlineGrid {
	return &InlineGrid{columns: columns}
}

// Add adds buttons after the last ones, starting new rows as needed.
func (g *InlineGrid) Add(buttons ...InlineKeyboardButton) *InlineGrid {
	for _, b := range buttons {
		last := len(g.rows) - 1
		if !g.open || g.columns >= 1 && len(g.rows[last]) >= g.columns {
			g.rows = append(g.rows, nil)
			g.open, last = true, last+1
		}
		g.rows[last] = append(g.rows[last], b)
	}
	return g
}

// Row adds buttons as a row of their own; the next Add starts a new row.
func (g *InlineGrid) Row(buttons ...InlineKeyboardButton) *InlineGrid {
	if len(buttons) > 0 {
		g.rows = append(g.rows, buttons)
	}
	g.open = false
	return g
}

// Markup returns the keyboard to send with a message.
func (g *InlineGrid) Markup() *InlineKeyboardMarkup {
	rows := make([][]InlineKeyboardButton, len(g.rows))
	for i, r := range g.rows {
		rows[i] = slices.Clone(r)
	}
	return NewInlineKeyboard(rows...)
}
