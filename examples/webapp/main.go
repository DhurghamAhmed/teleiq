// Command webapp opens a pizza Mini App from /start and sends the order to the chat.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/webapp"
)

func main() {
	// ctx ends on Ctrl+C or SIGTERM, which stops the bot.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// ln listens on ADDR, or on :8080 when ADDR is empty.
	ln, err := net.Listen("tcp", cmp.Or(os.Getenv("ADDR"), ":8080"))
	if err == nil {
		err = run(ctx, os.Getenv("BOT_TOKEN"), ln, os.Getenv("WEBAPP_URL"))
	}
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// pizzas are the orders that the server accepts.
var pizzas = []string{"Margherita", "Pepperoni", "Veggie"}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, ln net.Listener, pageURL string, opts ...teleiq.Option) error {
	if pageURL == "" {
		_ = ln.Close()
		return errors.New("set WEBAPP_URL, the HTTPS address at which Telegram opens the page")
	}
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		_ = ln.Close()
		return err
	}
	b := bot.New(client)  // b is the bot
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	b.OnCommand("start", func(ctx context.Context, c *bot.Context) error {
		// c is the current update; the button opens the page.
		button := models.InlineKeyboardButton{Text: "Order a pizza", WebApp: &models.WebAppInfo{URL: pageURL}}
		return c.Send(ctx, "What would you like?", bot.Keyboard(models.NewInlineKeyboard(models.NewInlineRow(button))))
	})

	// mux routes GET / to the page and POST /order to order.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
	mux.Handle("POST /order", order(client, token))
	// srv serves the page and the orders; served gets its result.
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	err = b.Run(ctx) // receives updates until ctx ends
	// Longer than the 5s after which Shutdown drops idle connections.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := srv.Shutdown(shutdown); err == nil {
		err = shutdownErr
	}
	if serveErr := <-served; err == nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = serveErr
	}
	return err
}

// order checks the init data of an order and sends the order as the user.
func order(client *teleiq.Client, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// req is the body that the page posts.
		var req struct {
			InitData string `json:"init_data"`
			Pizza    string `json:"pizza"`
		}
		// 64<<10 is 64 KiB, the most that a request may send.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || !slices.Contains(pizzas, req.Pizza) {
			http.Error(w, "bad order", http.StatusBadRequest)
			return
		}
		// data is the init data, trusted only if Telegram signed it for this bot.
		data, err := webapp.Validate(req.InitData, token, time.Hour)
		if err != nil {
			http.Error(w, "not from Telegram", http.StatusForbidden)
			return
		}
		if data.QueryID == "" || data.User == nil {
			http.Error(w, "open the page from its button", http.StatusBadRequest)
			return
		}
		_, err = client.AnswerWebAppQuery(r.Context(), teleiq.AnswerWebAppQueryParams{
			WebAppQueryID: data.QueryID,
			Result: &models.InlineQueryResultArticle{
				ID:                  "order",
				Title:               "Order",
				InputMessageContent: &models.InputTextMessageContent{MessageText: data.User.FirstName + " ordered a " + req.Pizza + " pizza."},
			},
		})
		if err != nil {
			http.Error(w, "the order was not sent", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// page is the Mini App: it posts its init data and the pizza picked.
const page = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pizza</title>
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<style>
body { font-family: system-ui, sans-serif; margin: 16px; background: var(--tg-theme-bg-color, #fff); color: var(--tg-theme-text-color, #000); }
button { display: block; width: 100%; margin: 8px 0; padding: 14px; font-size: 17px; border: 0; border-radius: 10px;
  background: var(--tg-theme-button-color, #2481cc); color: var(--tg-theme-button-text-color, #fff); }
</style>
</head>
<body>
<h3>Pick a pizza</h3>
<button onclick="send('Margherita')">Margherita</button>
<button onclick="send('Pepperoni')">Pepperoni</button>
<button onclick="send('Veggie')">Veggie</button>
<p id="status"></p>
<script>
const app = window.Telegram.WebApp;
app.ready();
async function send(pizza) {
  const res = await fetch("order", {method: "POST", headers: {"Content-Type": "application/json"},
    body: JSON.stringify({init_data: app.initData, pizza: pizza})});
  if (res.ok) { app.close(); } else { document.getElementById("status").textContent = await res.text(); }
}
</script>
</body>
</html>
`
