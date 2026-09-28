package main

import (
	"context"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

func TestWorkFinishesDuringShutdown(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	stop := teleiqtest.Start(t, func(ctx context.Context) error {
		return run(ctx, teleiqtest.Token, 200*time.Millisecond, teleiq.WithBaseURL(srv.URL))
	})

	srv.Push(teleiqtest.MessageUpdate(7, "/work"))
	srv.Wait(t, "sendMessage", 1) // "Working..."
	start := time.Now()
	if err := stop(); err != nil {
		t.Fatalf("run() = %v, want a clean stop", err)
	}
	sent := srv.Wait(t, "sendMessage", 2)
	if sent[1].Params["text"] != "Done." || time.Since(start) < 100*time.Millisecond {
		t.Errorf("after the stop sent %v after %v, want the work finished first", sent[1].Params, time.Since(start))
	}
}
