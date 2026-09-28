package bot

import (
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

func chatUpdate(chat, seq int64) *models.Update {
	return &models.Update{UpdateID: seq, Message: &models.Message{MessageID: seq, Date: 1, Chat: models.Chat{ID: chat}}}
}

// TestSchedulerKeepsTheOrderOfEachChat queues every update at once and makes the earlier updates
// of a chat slower than its later ones, so only running one update of a chat at a time keeps them
// in order.
func TestSchedulerKeepsTheOrderOfEachChat(t *testing.T) {
	const chats, perChat = 40, 20
	s := newScheduler(4, chats*perChat)
	var (
		mu        sync.Mutex
		seen      = map[int64][]int64{}
		active    = map[int64]int{}
		overlap   bool
		running   atomic.Int32
		parallel  atomic.Int32
		completed sync.WaitGroup
	)
	s.start(context.Background(), func(_ context.Context, u *models.Update) {
		chat := u.Message.Chat.ID
		mu.Lock()
		active[chat]++
		overlap = overlap || active[chat] > 1
		mu.Unlock()
		n := running.Add(1)
		for {
			p := parallel.Load()
			if n <= p || parallel.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(time.Duration(perChat-u.Message.MessageID)*50*time.Microsecond + time.Duration(rand.IntN(20))*time.Microsecond)
		running.Add(-1)
		mu.Lock()
		active[chat]--
		seen[chat] = append(seen[chat], u.Message.MessageID)
		mu.Unlock()
	})

	for seq := range int64(perChat) {
		for chat := range int64(chats) {
			completed.Add(1)
			if err := s.submit(context.Background(), job{update: chatUpdate(chat+1, seq), done: func(*models.Update) { completed.Done() }}); err != nil {
				t.Fatalf("submit() error = %v", err)
			}
		}
	}
	completed.Wait()
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	for chat := range int64(chats) {
		got := seen[chat+1]
		if len(got) != perChat {
			t.Fatalf("chat %d: %d updates handled, want %d", chat+1, len(got), perChat)
		}
		for i, seq := range got {
			if seq != int64(i) {
				t.Fatalf("chat %d handled its updates in the order %v", chat+1, got)
			}
		}
	}
	if overlap {
		t.Error("two updates of one chat were handled at the same time")
	}
	if parallel.Load() < 2 {
		t.Errorf("at most %d updates ran at once, want different chats in parallel", parallel.Load())
	}
}

func TestSchedulerBackpressure(t *testing.T) {
	s := newScheduler(1, 2)
	started, release := make(chan struct{}, 10), make(chan struct{})
	var handled atomic.Int32
	s.start(context.Background(), func(context.Context, *models.Update) {
		started <- struct{}{}
		<-release
		handled.Add(1)
	})
	ctx := context.Background()

	if err := s.submit(ctx, job{update: chatUpdate(1, 1)}); err != nil {
		t.Fatal(err)
	}
	<-started // the worker holds the first update; the chat has room for two more waiting
	for seq := range int64(2) {
		if err := s.submit(ctx, job{update: chatUpdate(1, seq+2)}); err != nil {
			t.Fatal(err)
		}
	}

	short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.submit(short, job{update: chatUpdate(1, 4)}); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) < 25*time.Millisecond {
		t.Errorf("submit() on a full chat = %v after %v, want to wait until the deadline", err, time.Since(start))
	}
	if err := s.trySubmit(job{update: chatUpdate(1, 4)}); !errors.Is(err, ErrQueueFull) {
		t.Errorf("trySubmit() on a full chat = %v, want ErrQueueFull", err)
	}

	close(release)
	if err := s.submit(ctx, job{update: chatUpdate(1, 4)}); err != nil {
		t.Errorf("submit() after the chat drained = %v", err)
	}
	s.close()
	if err := s.wait(ctx); err != nil || handled.Load() != 4 {
		t.Errorf("wait() = %v with %d updates handled, want 4", err, handled.Load())
	}
}

func TestSchedulerClose(t *testing.T) {
	s := newScheduler(2, 16)
	release := make(chan struct{})
	s.start(context.Background(), func(context.Context, *models.Update) { <-release })
	var done atomic.Int32
	for seq := range int64(10) {
		if err := s.submit(context.Background(), job{update: chatUpdate(seq, seq), done: func(*models.Update) { done.Add(1) }}); err != nil {
			t.Fatal(err)
		}
	}

	s.close()
	s.close()

	if err := s.submit(context.Background(), job{update: chatUpdate(1, 1)}); !errors.Is(err, errClosed) {
		t.Errorf("submit() after close = %v, want errClosed", err)
	}
	if err := s.trySubmit(job{update: chatUpdate(1, 1)}); !errors.Is(err, errClosed) {
		t.Errorf("trySubmit() after close = %v, want errClosed", err)
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("wait() while handlers run = %v, want the deadline of its context", err)
	}
	close(release)
	if err := s.wait(context.Background()); err != nil || done.Load() != 10 {
		t.Errorf("wait() = %v with %d updates done, want every queued update handled", err, done.Load())
	}
}

func TestSchedulerPassesItsContext(t *testing.T) {
	type key struct{}
	s := newScheduler(1, 1)
	got := make(chan any, 1)
	s.start(context.WithValue(context.Background(), key{}, "run"), func(ctx context.Context, _ *models.Update) {
		got <- ctx.Value(key{})
	})
	if err := s.submit(context.Background(), job{update: chatUpdate(1, 1)}); err != nil {
		t.Fatal(err)
	}
	if v := <-got; v != "run" {
		t.Errorf("handler context value = %v, want the one given to start", v)
	}
	s.close()
	_ = s.wait(context.Background())
}

func TestSchedulerStopsItsGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	s := newScheduler(8, 4)
	s.start(context.Background(), func(context.Context, *models.Update) {})
	for seq := range int64(20) {
		if err := s.submit(context.Background(), job{update: chatUpdate(seq, seq)}); err != nil {
			t.Fatal(err)
		}
	}
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines after the scheduler stopped, want at most %d", n, before)
	}
}
