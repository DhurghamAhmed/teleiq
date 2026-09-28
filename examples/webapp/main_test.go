package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestWebApp(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		return run(ctx, teleiqtest.Token, ln, "https://pizza.example.com/", teleiq.WithBaseURL(srv.URL))
	})
	base := "http://" + ln.Addr().String()
	// No keep-alives, so no idle connection makes stopping wait 5 seconds.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	srv.WaitHandled(t, srv.Push(teleiqtest.MessageUpdate(7, "/start")))
	if markup := fmt.Sprint(srv.Requests("sendMessage")[0].Params["reply_markup"]); !strings.Contains(markup, "web_app:map[url:https://pizza.example.com/]") {
		t.Errorf("reply_markup = %s, want a button that opens the Mini App", markup)
	}

	resp, err := client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "telegram-web-app.js") {
		t.Errorf("GET / = %d, want the page", resp.StatusCode)
	}

	initData := func(fields url.Values) string { return teleiqtest.WebAppInitData(teleiqtest.Token, fields) }
	valid := initData(url.Values{"query_id": {"AAQ1"}, "user": {`{"id":7,"first_name":"User7"}`}})
	tests := []struct {
		name     string
		initData string
		pizza    string
		want     int
	}{
		{"order", valid, "Pepperoni", http.StatusNoContent},
		{"changed on the way", strings.Replace(valid, "User7", "Admin", 1), "Pepperoni", http.StatusForbidden},
		{"not from this bot", teleiqtest.WebAppInitData("999:other", url.Values{"query_id": {"AAQ1"}}), "Veggie", http.StatusForbidden},
		{"expired", initData(url.Values{"query_id": {"AAQ1"}, "auth_date": {strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)}}),
			"Veggie", http.StatusForbidden},
		{"no query", initData(url.Values{"user": {`{"id":7,"first_name":"User7"}`}}), "Veggie", http.StatusBadRequest},
		{"unknown pizza", valid, "Hawaiian", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := json.Marshal(map[string]string{"init_data": tt.initData, "pizza": tt.pizza})
			resp, err := client.Post(base+"/order", "application/json", strings.NewReader(string(req)))
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("POST /order = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}

	answers := srv.Requests("answerWebAppQuery")
	if len(answers) != 1 {
		t.Fatalf("%d calls to answerWebAppQuery, want only the valid order", len(answers))
	}
	p := answers[0].Params
	if text := fmt.Sprint(p["result"]); p["web_app_query_id"] != "AAQ1" || !strings.Contains(text, "message_text:User7 ordered a Pepperoni pizza.") {
		t.Errorf("answerWebAppQuery = %v", p)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

// A connection that never sends a request must not make run fail as it stops.
func TestStopWithUnusedConnection(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		return run(ctx, teleiqtest.Token, ln, "https://pizza.example.com/", teleiq.WithBaseURL(srv.URL))
	})

	unused, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unused.Close() }()
	// Connections are accepted in order, so the unused one is accepted by now.
	resp, err := (&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}).Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if err := stop(); err != nil {
		t.Errorf("run() = %v, want nil", err)
	}
}
