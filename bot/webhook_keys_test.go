package bot

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestWebhookChatsDoNotWaitForEachOther: requests of chats 5, 6, 7 and 12, which shared a worker
// with chat 4 before updates were scheduled by chat, are answered while the handler of chat 4 is
// stuck.
func TestWebhookChatsDoNotWaitForEachOther(t *testing.T) {
	b, post := newWebhookBot(t)
	started, release := make(chan struct{}), make(chan struct{})
	// Before the server closes, which waits for the request of chat 4, even when the test fails.
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	b.OnMessage(func(_ context.Context, c *Context) error {
		if c.Chat().ID == 4 {
			close(started)
			<-release
		}
		return nil
	})
	stop := startWebhook(t, b)
	stuck := postAsync(post, update(1, 4))
	<-started
	for i, chat := range []int64{5, 6, 7, 12} {
		select {
		case code := <-postAsync(post, update(int64(i+2), chat)):
			if code != http.StatusOK {
				t.Errorf("chat %d: %d, want 200", chat, code)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("chat %d not answered while the handler of chat 4 was stuck", chat)
		}
	}
	unblock()
	if code := <-stuck; code != http.StatusOK {
		t.Errorf("chat 4: %d, want 200", code)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

// TestWebhookFullChat: a chat that fills its queue gets 503 and ErrQueueFull, and another chat
// is still accepted.
func TestWebhookFullChat(t *testing.T) {
	var mu sync.Mutex
	var reported []error
	b, post, started, release := blockingBot(t, WithAsyncWebhook(), WithWorkers(2), WithQueueSize(2),
		WithErrorHandler(func(_ context.Context, c *Context, err error) {
			if c == nil {
				mu.Lock()
				reported = append(reported, err)
				mu.Unlock()
			}
		}))
	stop := startWebhook(t, b)
	post(update(1, 1), "")
	<-started // chat 1 runs update 1
	for id := int64(2); id <= 3; id++ {
		if code := post(update(id, 1), ""); code != http.StatusOK {
			t.Errorf("waiting update %d of chat 1: %d, want 200", id, code)
		}
	}
	if code := post(update(4, 1), ""); code != http.StatusServiceUnavailable {
		t.Errorf("third waiting update of chat 1: %d, want 503", code)
	}
	if code := post(update(5, 2), ""); code != http.StatusOK {
		t.Errorf("chat 2 while chat 1 is full: %d, want 200", code)
	}
	close(release)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reported) != 1 || !errors.Is(reported[0], ErrQueueFull) {
		t.Errorf("reported %v, want ErrQueueFull once", reported)
	}
}
