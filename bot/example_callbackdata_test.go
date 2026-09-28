package bot_test

import (
	"context"
	"fmt"
	"log"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// Callback builds the data of buttons that carry several values, here a product and a quantity,
// and reads them back in the handler of their presses.
func ExampleCallback() {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client)

	buy := bot.NewCallback("buy")
	keyboard := models.NewInlineKeyboard(models.NewInlineRow(buy.Button("Buy 1", int64(42), 1), buy.Button("Buy 5", int64(42), 5)))
	handler := func(ctx context.Context, c *bot.Context) error {
		f := buy.Unpack(c.CallbackData())
		id, qty := f.Int64(0), f.Int(1)
		if f.Err() != nil {
			return c.Answer(ctx, "This button is out of date.")
		}
		return c.Answer(ctx, fmt.Sprintf("Bought %d of product %d", qty, id))
	}
	b.Handle(buy, handler)

	data := *keyboard.InlineKeyboard[0][1].CallbackData
	u := teleiqtest.CallbackUpdate(7, 1, data)
	fmt.Println(data, buy.Match(b.NewContext(&u)))
	if err := handler(context.Background(), b.NewContext(&u)); err != nil {
		log.Fatal(err)
	}
	fmt.Println(srv.Requests("answerCallbackQuery")[0].Params["text"])

	_, err = buy.Pack("a note far too long for the 64 bytes that Telegram allows in callback data")
	fmt.Println(err)
	// Output:
	// buy:42:5 true
	// Bought 5 of product 42
	// bot: Callback.Pack: 78 bytes of callback data; Telegram allows 1 to 64
}
