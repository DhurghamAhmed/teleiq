package bot

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DhurghamAhmed/teleiq/models"
)

// TestSchedulerKeysDoNotWaitForEachOther: with fixed lanes, chats 4, 5, 6, 7 and 12 shared one of
// 8 workers, so a stuck handler in chat 4 held back the others; now none of them waits.
func TestSchedulerKeysDoNotWaitForEachOther(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newScheduler(8, 128)
		var mu sync.Mutex
		handled := map[int64]time.Duration{}
		start := time.Now()
		s.start(context.Background(), func(_ context.Context, u *models.Update) {
			if u.Message.Chat.ID == 4 {
				time.Sleep(time.Minute)
			}
			mu.Lock()
			handled[u.Message.Chat.ID] = time.Since(start)
			mu.Unlock()
		})
		for _, chat := range []int64{4, 5, 6, 7, 12} {
			if err := s.submit(context.Background(), job{update: chatUpdate(chat, chat)}); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()
		mu.Lock()
		for _, chat := range []int64{5, 6, 7, 12} {
			if d, ok := handled[chat]; !ok || d != 0 {
				t.Errorf("chat %d handled after %v (handled: %v), want at once", chat, d, ok)
			}
		}
		mu.Unlock()
		s.close()
		if err := s.wait(context.Background()); err != nil || handled[4] != time.Minute {
			t.Errorf("wait() = %v with chat 4 handled after %v, want nil after a minute", err, handled[4])
		}
	})
}

// TestSchedulerLimits: a key holds perKey waiting updates, all keys together workers*perKey, and
// the updates running do not count.
func TestSchedulerLimits(t *testing.T) {
	s := newScheduler(2, 3) // 6 waiting at most
	started, release := make(chan int64, 16), make(chan struct{})
	s.start(context.Background(), func(_ context.Context, u *models.Update) {
		started <- u.Message.Chat.ID
		<-release
	})
	try := func(chat, seq int64) error { return s.trySubmit(job{update: chatUpdate(chat, seq)}) }
	for chat := int64(1); chat <= 2; chat++ { // both workers busy
		if err := try(chat, 0); err != nil {
			t.Fatal(err)
		}
		<-started
	}
	for seq := range int64(3) {
		if err := try(1, seq+1); err != nil {
			t.Fatalf("waiting update %d of chat 1: %v", seq+1, err)
		}
	}
	if err := try(1, 4); !errors.Is(err, ErrQueueFull) {
		t.Errorf("fourth waiting update of chat 1 = %v, want ErrQueueFull", err)
	}
	for chat := int64(3); chat <= 5; chat++ {
		if err := try(chat, 1); err != nil {
			t.Fatalf("chat %d, whose own queue is empty, got %v", chat, err)
		}
	}
	if err := try(6, 1); !errors.Is(err, ErrQueueFull) {
		t.Errorf("seventh waiting update = %v, want ErrQueueFull", err)
	}
	close(release)
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestSchedulerFairness: with one worker, a key that became ready during the turn of A runs before
// what A received after that.
func TestSchedulerFairness(t *testing.T) {
	s := newScheduler(1, 16)
	release := make(chan struct{})
	var order []int64
	s.start(context.Background(), func(_ context.Context, u *models.Update) {
		if u.UpdateID == 1 {
			<-release
		}
		order = append(order, u.UpdateID)
	})
	submit := func(chat, id int64) {
		if err := s.submit(context.Background(), job{update: chatUpdate(chat, id)}); err != nil {
			t.Fatal(err)
		}
	}
	submit(1, 1)
	eventually(t, "the turn of chat 1", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.waiting == 0 })
	submit(2, 2) // chat 2 becomes ready during the turn
	submit(1, 3) // then chat 1 receives update 3
	close(release)
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, 2, 3}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// TestSchedulerModel has four producers submit random updates to keys of their own while handlers
// take random times, and checks the promises of the scheduler.
func TestSchedulerModel(t *testing.T) {
	for _, workers := range []int{1, 3, 8} {
		const keys, total = 40, 5000
		s := newScheduler(workers, 4)
		var (
			mu      sync.Mutex
			running = map[int64]bool{}
			last    = map[int64]int64{}
			handled = map[int64]int{}
			failed  error
			done    atomic.Int64
			active  atomic.Int32
			peak    atomic.Int32
		)
		s.start(context.Background(), func(_ context.Context, u *models.Update) {
			key, seq := u.Message.Chat.ID, u.Message.MessageID
			mu.Lock()
			if running[key] {
				failed = errors.New("two updates of a key at once")
			}
			if seq <= last[key] {
				failed = errors.New("updates of a key out of order")
			}
			running[key], last[key] = true, seq
			handled[u.UpdateID]++
			mu.Unlock()
			n := active.Add(1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			time.Sleep(time.Duration(rand.IntN(50)) * time.Microsecond)
			active.Add(-1)
			mu.Lock()
			running[key] = false
			mu.Unlock()
		})
		var wg sync.WaitGroup
		var id atomic.Int64
		for p := range 4 {
			wg.Go(func() {
				next := map[int]int64{}
				for range total / 4 {
					k := rand.IntN(keys/4)*4 + p
					next[k]++
					u := chatUpdate(int64(k+1), next[k])
					u.UpdateID = id.Add(1)
					if err := s.submit(context.Background(), job{update: u, done: func(*models.Update) { done.Add(1) }}); err != nil {
						t.Error(err)
						return
					}
				}
			})
		}
		wg.Wait()
		s.close()
		if err := s.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		if failed != nil {
			t.Errorf("workers %d: %v", workers, failed)
		}
		if done.Load() != total || len(handled) != total {
			t.Errorf("workers %d: %d done and %d handled, want %d", workers, done.Load(), len(handled), total)
		}
		for id, n := range handled {
			if n != 1 {
				t.Errorf("workers %d: update %d handled %d times", workers, id, n)
			}
		}
		if p := peak.Load(); p > int32(workers) {
			t.Errorf("workers %d: %d handlers at once", workers, p)
		}
		if len(s.keys) != 0 || s.waiting != 0 {
			t.Errorf("workers %d: %d keys and %d updates left", workers, len(s.keys), s.waiting)
		}
	}
}

// TestSchedulerManyKeys: the keys held stay under workers*perKey+workers however many there are.
func TestSchedulerManyKeys(t *testing.T) {
	const workers, perKey = 8, 16
	s := newScheduler(workers, perKey)
	var peak int
	s.start(context.Background(), func(context.Context, *models.Update) {
		s.mu.Lock()
		peak = max(peak, len(s.keys))
		s.mu.Unlock()
	})
	for key := range int64(200_000) {
		if err := s.submit(context.Background(), job{update: chatUpdate(key+1, 1)}); err != nil {
			t.Fatal(err)
		}
	}
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if peak > workers*perKey+workers || len(s.keys) != 0 {
		t.Errorf("peak of %d keys and %d left, want at most %d and none", peak, len(s.keys), workers*perKey+workers)
	}
}

// TestSchedulerKeepsSmallArrays: a key that received a burst keeps no large array once it is gone.
func TestSchedulerKeepsSmallArrays(t *testing.T) {
	s := newScheduler(1, 128)
	release := make(chan struct{})
	s.start(context.Background(), func(_ context.Context, u *models.Update) {
		if u.UpdateID == 0 {
			<-release
		}
	})
	for seq := range int64(101) { // update 0 runs while 100 wait
		if err := s.submit(context.Background(), job{update: chatUpdate(1, seq)}); err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	s.close()
	if err := s.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, k := range s.free {
		if cap(k.pending) > keepCap || cap(k.spare) > keepCap {
			t.Errorf("a deleted key keeps arrays of %d and %d, want at most %d", cap(k.pending), cap(k.spare), keepCap)
		}
	}
}

// TestSchedulerSubmitWakesWhenItsContextEnds: a submit that waits for room returns when its context
// ends, not later.
func TestSchedulerSubmitWakesWhenItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newScheduler(1, 1)
		release := make(chan struct{})
		s.start(context.Background(), func(context.Context, *models.Update) { <-release })
		ctx := context.Background()
		for seq := range int64(2) { // one runs, one waits
			if err := s.submit(ctx, job{update: chatUpdate(1, seq)}); err != nil {
				t.Fatal(err)
			}
		}
		short, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		start := time.Now()
		if err := s.submit(short, job{update: chatUpdate(1, 3)}); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != time.Second {
			t.Errorf("submit() = %v after %v, want the deadline after 1s", err, time.Since(start))
		}
		close(release)
		s.close()
		if err := s.wait(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
