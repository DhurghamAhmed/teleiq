package bot_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// An ErrorHandler also receives the webhook requests that were rejected, with a nil Context and an
// error to match with errors.Is.
func ExampleWithErrorHandler() {
	client, err := teleiq.NewClient(teleiqtest.Token)
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client, bot.WithSecretToken("s3cret"), bot.WithErrorHandler(func(_ context.Context, c *bot.Context, err error) {
		switch {
		case c != nil:
			fmt.Println("update", c.Update().UpdateID, "failed:", err)
		case errors.Is(err, bot.ErrBadSecret):
			fmt.Println("a request without the secret token")
		case errors.Is(err, bot.ErrNotRunning):
			fmt.Println("an update while RunWebhook is not running; Telegram sends it again")
		default:
			fmt.Println(err)
		}
	}))

	webhook := b.WebhookHandler()
	for _, secret := range []string{"guess", "s3cret"} {
		req := httptest.NewRequest(http.MethodPost, "/telegram", strings.NewReader(`{"update_id":1}`))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		rec := httptest.NewRecorder()
		webhook.ServeHTTP(rec, req)
		fmt.Println(rec.Code)
	}
	// Output:
	// a request without the secret token
	// 401
	// an update while RunWebhook is not running; Telegram sends it again
	// 503
}
