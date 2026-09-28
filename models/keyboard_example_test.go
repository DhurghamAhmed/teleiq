package models_test

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func ExampleNewInlineKeyboard() {
	p := teleiq.SendMessageParams{
		ChatID: models.ID(42),
		Text:   "Pick a color:",
		ReplyMarkup: models.NewInlineKeyboard(
			models.NewInlineRow(
				models.NewCallbackButton("Red", "color:red"),
				models.NewCallbackButton("Blue", "color:blue"),
			),
			models.NewInlineRow(models.NewURLButton("Help", "https://example.com/help")),
		),
	}
	markup, err := json.Marshal(p.ReplyMarkup)
	fmt.Println(string(markup), err)
	// Output:
	// {"inline_keyboard":[[{"text":"Red","callback_data":"color:red"},{"text":"Blue","callback_data":"color:blue"}],[{"text":"Help","url":"https://example.com/help"}]]} <nil>
}

// NewInlineGrid builds a keyboard from a list, here products in rows of two with a back button
// under them.
func ExampleNewInlineGrid() {
	products := []string{"Tea", "Coffee", "Juice", "Water", "Milk"}

	grid := models.NewInlineGrid(2)
	for i, name := range products {
		grid.Add(models.NewCallbackButton(name, fmt.Sprintf("show:%d", i)))
	}
	grid.Row(models.NewCallbackButton("« Back", "back"))

	for _, row := range grid.Markup().InlineKeyboard {
		var texts []string
		for _, b := range row {
			texts = append(texts, "["+b.Text+"]")
		}
		fmt.Println(strings.Join(texts, " "))
	}
	// Output:
	// [Tea] [Coffee]
	// [Juice] [Water]
	// [Milk]
	// [« Back]
}
