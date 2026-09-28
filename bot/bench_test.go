package bot

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

func noop(context.Context, *Context) error { return nil }

// BenchmarkDispatch measures the framework's work for one update besides scheduling it: building
// its Context, running middleware and choosing its handler among several.
func BenchmarkDispatch(b *testing.B) {
	bt, _ := newTestBot(b, newFakeTelegram())
	bt.Use(Recovery())
	bt.OnCommand("start", noop)
	bt.OnCallback(noop)
	bt.OnMessage(noop)
	u := &models.Update{UpdateID: 1, Message: &models.Message{MessageID: 1, Date: 1,
		Chat: models.Chat{ID: 7, Type: "private"}, From: &models.User{ID: 7, FirstName: "Ann"}, Text: teleiq.Ptr("hello")}}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		bt.dispatch(ctx, u)
	}
}

// BenchmarkDispatchGroup is BenchmarkDispatch through a group with a filter and a middleware, the
// cost that a bot pays for a group; a bot without groups pays nothing.
func BenchmarkDispatchGroup(b *testing.B) {
	bt, _ := newTestBot(b, newFakeTelegram())
	bt.Use(Recovery())
	g := bt.Group(FilterFunc(func(c *Context) bool { return c.Chat() != nil && c.Chat().Type == "private" }))
	g.Use(func(next Handler) Handler { return next })
	g.OnCommand("start", noop)
	g.OnCallback(noop)
	g.OnMessage(noop)
	u := &models.Update{UpdateID: 1, Message: &models.Message{MessageID: 1, Date: 1,
		Chat: models.Chat{ID: 7, Type: "private"}, From: &models.User{ID: 7, FirstName: "Ann"}, Text: teleiq.Ptr("hello")}}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		bt.dispatch(ctx, u)
	}
}

// BenchmarkScheduling hands updates to the workers with a handler that does nothing, so it measures
// the scheduling alone: for one busy chat, and for updates spread over many chats.
func BenchmarkScheduling(b *testing.B) {
	for _, chats := range []int{1, 1000} {
		b.Run(fmt.Sprintf("chats=%d", chats), func(b *testing.B) {
			updates := make([]models.Update, chats)
			for i := range updates {
				updates[i].Message = &models.Message{Chat: models.Chat{ID: int64(i + 1)}}
			}
			ctx := context.Background()
			s := newScheduler(8, 128)
			s.start(ctx, func(context.Context, *models.Update) {})
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				if err := s.submit(ctx, job{update: &updates[i%chats]}); err != nil {
					b.Fatal(err)
				}
			}
			s.close()
			if err := s.wait(ctx); err != nil {
				b.Fatal(err)
			}
		})
	}
}

// batchTelegram answers every getUpdates at once with the next 100 updates, spread over chats, so
// that a benchmark measures the bot rather than the fake.
type batchTelegram struct {
	chats int64
	mu    sync.Mutex
	next  int64
}

func (f *batchTelegram) Do(_ context.Context, req *teleiq.Request) (*teleiq.Response, error) {
	if req.Method == "getMe" {
		return &teleiq.Response{StatusCode: 200, Body: []byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)}, nil
	}
	f.mu.Lock()
	first := f.next + 1
	f.next += 100
	f.mu.Unlock()
	body := make([]byte, 0, 100*110)
	body = append(body, `{"ok":true,"result":[`...)
	for id := first; id < first+100; id++ {
		if id > first {
			body = append(body, ',')
		}
		body = append(body, `{"update_id":`...)
		body = strconv.AppendInt(body, id, 10)
		body = append(body, `,"message":{"message_id":`...)
		body = strconv.AppendInt(body, id, 10)
		body = append(body, `,"date":1,"chat":{"id":`...)
		body = strconv.AppendInt(body, id%f.chats+1, 10)
		body = append(body, `,"type":"private"},"text":"hi"}}`...)
	}
	body = append(body, "]}"...)
	return &teleiq.Response{StatusCode: 200, Body: body}, nil
}

// BenchmarkPolling runs a bot with long polling until it has handled b.N updates, arriving in full
// batches: decoding, dispatching and scheduling together.
func BenchmarkPolling(b *testing.B) {
	for _, chats := range []int64{1, 1000} {
		b.Run(fmt.Sprintf("chats=%d", chats), func(b *testing.B) {
			client, err := teleiq.NewClient(testToken, teleiq.WithTransport(&batchTelegram{chats: chats}))
			if err != nil {
				b.Fatal(err)
			}
			bt := New(client, WithPollTimeout(time.Second))
			var handled atomic.Int64
			done := make(chan struct{})
			bt.OnMessage(func(context.Context, *Context) error {
				if handled.Add(1) == int64(b.N) {
					close(done)
				}
				return nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			b.ReportAllocs()
			b.ResetTimer()
			go func() { result <- bt.Run(ctx) }()
			<-done
			b.StopTimer()
			cancel()
			if err := <-result; err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "updates/s")
		})
	}
}

// BenchmarkWebhook posts updates one after another to the webhook, which answers each once its
// handler has returned.
func BenchmarkWebhook(b *testing.B) {
	bt, _ := newTestBot(b, newFakeTelegram())
	bt.OnMessage(noop)
	stop := startWebhook(b, bt)
	h := bt.WebhookHandler()
	body := []byte(update(1, 7))
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
	b.StopTimer()
	if err := stop(); err != nil {
		b.Fatal(err)
	}
}
