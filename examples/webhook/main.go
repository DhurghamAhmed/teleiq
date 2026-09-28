// Command webhook receives updates through a webhook at WEBHOOK_URL instead of polling.
package main

import (
	"cmp"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
)

func main() {
	// ctx ends on Ctrl+C or SIGTERM, which stops the bot.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// ln listens on ADDR, or on :8080 when ADDR is empty.
	ln, err := net.Listen("tcp", cmp.Or(os.Getenv("ADDR"), ":8080"))
	if err == nil {
		err = run(ctx, os.Getenv("BOT_TOKEN"), ln, os.Getenv("WEBHOOK_URL"), os.Getenv("WEBHOOK_SECRET"))
	}
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// run is the whole program; its test passes a fake Bot API server in opts.
func run(ctx context.Context, token string, ln net.Listener, url, secret string, opts ...teleiq.Option) error {
	if url == "" || secret == "" {
		_ = ln.Close()
		return errors.New("set WEBHOOK_URL and WEBHOOK_SECRET, a secret of letters, digits, _ and -")
	}
	client, err := teleiq.NewClient(token, opts...)
	if err != nil {
		_ = ln.Close()
		return err
	}
	// b is the bot; it accepts only the requests that carry the secret.
	b := bot.New(client, bot.WithSecretToken(secret))
	b.Use(bot.Recovery()) // a panic in a handler does not stop the bot
	b.OnMessage(func(ctx context.Context, c *bot.Context) error {
		// c is the current update; c.Send replies in its chat.
		return c.Send(ctx, "Received through a webhook.")
	})

	// srv passes Telegram's requests to the bot and closes ln when it stops.
	srv := &http.Server{Handler: b.WebhookHandler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	// Tells Telegram where to send updates; [] asks for the default kinds.
	err = client.SetWebhook(ctx, teleiq.SetWebhookParams{URL: url, SecretToken: &secret, AllowedUpdates: []string{}})
	if err == nil {
		err = b.RunWebhook(ctx) // handles the updates until ctx ends
	}
	// RunWebhook has answered every request, so the server can stop.
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if shutdownErr := srv.Shutdown(shutdown); err == nil {
		err = shutdownErr
	}
	if serveErr := <-served; err == nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = serveErr
	}
	return err
}
