package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

const secret = "test_secret-1"

func TestWebhook(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		return run(ctx, teleiqtest.Token, ln, "https://bot.example.com/telegram", secret, teleiq.WithBaseURL(srv.URL))
	})

	if p := srv.Wait(t, "setWebhook", 1)[0].Params; p["url"] != "https://bot.example.com/telegram" || p["secret_token"] != secret ||
		fmt.Sprint(p["allowed_updates"]) != "[]" {
		t.Errorf("setWebhook = %v", p)
	}
	u := teleiqtest.MessageUpdate(7, "hi")
	u.UpdateID, u.Message.MessageID, u.Message.Date = 1, 1, time.Now().Unix()
	body, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	post := func(secret string) int {
		req, _ := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String()+"/", bytes.NewReader(body))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	// RunWebhook starts right after setWebhook; until then the handler answers 503.
	deadline := time.Now().Add(5 * time.Second)
	for post(secret) != http.StatusOK {
		if time.Now().After(deadline) {
			t.Fatal("the webhook never accepted the update")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if p := srv.Wait(t, "sendMessage", 1)[0].Params; p["chat_id"] != 7.0 || p["text"] != "Received through a webhook." {
		t.Errorf("reply = %v", p)
	}
	if code := post("wrong"); code != http.StatusUnauthorized {
		t.Errorf("wrong secret: %d, want 401", code)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookNeedsItsSettings(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), "1:test", ln, "", ""); err == nil || !strings.Contains(err.Error(), "WEBHOOK_URL") {
		t.Errorf("run() = %v, want the missing settings", err)
	}
}
