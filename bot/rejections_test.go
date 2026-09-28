package bot

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// A bot with the default ErrorHandler and Logging, served by a real HTTP server with the fake Bot
// API behind it, shows what it rejects and what no handler matched.
func TestRejectionsReachTheLog(t *testing.T) {
	logs := captureLogAt(t, slog.LevelDebug)
	api := teleiqtest.NewServer()
	defer api.Close()
	client, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	const secret = "s3cret"
	b := New(client, WithSecretToken(secret))
	b.Use(Logging())
	b.OnCommand("start", func(context.Context, *Context) error { return nil })
	srv := httptest.NewServer(b.WebhookHandler())
	defer srv.Close()
	stop := teleiqtest.Start(t, b.RunWebhook)
	eventually(t, "RunWebhook to start", func() bool { return b.hook.Load() != nil })

	for _, r := range []struct {
		secret, body string
		want         int
	}{
		{"guess", update(1, 1), http.StatusUnauthorized},
		{"guess-again", update(2, 1), http.StatusUnauthorized},
		{secret, "{", http.StatusBadRequest},
		{secret, update(3, 1), http.StatusOK}, // no handler matches "hi"
	} {
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(r.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(secretHeader, r.secret)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != r.want {
			t.Errorf("status = %d, want %d", resp.StatusCode, r.want)
		}
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	text := logs.String()
	for _, want := range []string{
		`level=WARN msg="bot: webhook request rejected" error="bot: webhook request without the right secret token"`,
		`level=DEBUG msg="bot: webhook request rejected" error="bot: webhook request without the right secret token"`,
		`level=WARN msg="bot: webhook request rejected" error="bot: webhook request body is not an update"`,
		`level=INFO msg="bot: update handled" update_id=3 kind=message`,
		"handled=false",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, "guess") || strings.Contains(text, secret) {
		t.Errorf("the log contains a secret:\n%s", text)
	}
}
