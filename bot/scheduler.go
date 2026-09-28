package bot

import (
	"context"
	"errors"
	"sync"

	"github.com/DhurghamAhmed/teleiq/models"
)

var errClosed = errors.New("bot: not accepting updates")

// job is an update waiting for its turn.
type job struct {
	update *models.Update
	done   func(*models.Update) // called with update once it has been handled; may be nil
}

// scheduler runs updates on a pool of workers, in order within each routeKey.
type scheduler struct {
	workers, perKey, total int

	mu      sync.Mutex
	work    sync.Cond // signaled when a key becomes ready, broadcast at close
	space   sync.Cond // broadcast when updates start, at close, or when a waiter's ctx ends
	keys    map[int64]*keyQueue
	ready   ring // keys with waiting updates and no turn in progress, oldest first
	waiting int  // updates accepted and not started
	closed  bool
	free    []*keyQueue // queues of deleted keys, reused so that a new key does not allocate
	live    int         // workers that have not returned
	idle    int         // workers waiting for a ready key
	wakes   int         // signals sent to idle workers and not yet taken
	blocked int         // submits waiting for room

	stopped chan struct{} // closed when the last worker has returned
}

// keyQueue holds the waiting updates of one key.
type keyQueue struct {
	key     int64
	pending []job
	spare   []job // the array of the last turn, reused for the next one
	turn    bool  // a worker is running the updates of this key
}

// keepCap is the largest array a deleted key keeps for reuse.
const keepCap = 16

func newScheduler(workers, queueSize int) *scheduler {
	s := &scheduler{
		workers: workers, perKey: queueSize, total: workers * queueSize,
		keys: map[int64]*keyQueue{}, stopped: make(chan struct{}),
	}
	s.work.L, s.space.L = &s.mu, &s.mu
	return s
}

// start runs the workers, which handle updates until closed and nothing waits.
func (s *scheduler) start(ctx context.Context, handle func(context.Context, *models.Update)) {
	s.mu.Lock()
	s.live = s.workers
	s.mu.Unlock()
	for range s.workers {
		go s.worker(ctx, handle)
	}
}

func (s *scheduler) worker(ctx context.Context, handle func(context.Context, *models.Update)) {
	s.mu.Lock()
	for {
		for s.ready.len() == 0 && !s.closed {
			s.idle++
			s.work.Wait()
			s.idle--
			if s.wakes > 0 {
				s.wakes--
			}
		}
		if s.ready.len() == 0 {
			break
		}
		k := s.ready.pop()
		batch := k.pending
		k.pending, k.spare, k.turn = k.spare[:0], nil, true
		s.waiting -= len(batch)
		if s.blocked > 0 {
			s.space.Broadcast()
		}
		s.mu.Unlock()

		for _, j := range batch {
			// ctx ends at the shutdown timeout, after which Telegram sends the updates again.
			if ctx.Err() != nil {
				continue
			}
			handle(ctx, j.update)
			if j.done != nil {
				j.done(j.update)
			}
		}
		clear(batch)

		s.mu.Lock()
		k.turn, k.spare = false, batch[:0]
		if len(k.pending) > 0 {
			s.ready.push(k)
			continue
		}
		delete(s.keys, k.key)
		if cap(k.pending) > keepCap {
			k.pending = nil
		}
		if cap(k.spare) > keepCap {
			k.spare = nil
		}
		s.free = append(s.free, k)
	}
	s.live--
	if s.live == 0 {
		close(s.stopped)
	}
	s.mu.Unlock()
}

// submit accepts j, waiting while its key or all keys are full.
func (s *scheduler) submit(ctx context.Context, j job) error {
	return s.add(ctx, j, true)
}

// trySubmit accepts j, or fails at once with ErrQueueFull when there is no room.
func (s *scheduler) trySubmit(j job) error {
	return s.add(context.Background(), j, false)
}

// add accepts j, waiting for room while ctx lives if wait is set.
func (s *scheduler) add(ctx context.Context, j job, wait bool) error {
	key := routeKey(j.update)
	s.mu.Lock()
	defer s.mu.Unlock()
	var stop func() bool
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	for {
		if s.closed {
			return errClosed
		}
		k := s.keys[key]
		if (k == nil || len(k.pending) < s.perKey) && s.waiting < s.total {
			if k == nil {
				k = s.newKey(key)
			}
			k.pending = append(k.pending, j)
			s.waiting++
			if len(k.pending) == 1 && !k.turn {
				s.ready.push(k)
				// Waking a worker that another wake already covers would only fight over the lock.
				if s.idle > s.wakes {
					s.wakes++
					s.work.Signal()
				}
			}
			return nil
		}
		if !wait {
			return ErrQueueFull
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if stop == nil {
			stop = context.AfterFunc(ctx, func() {
				s.mu.Lock()
				s.space.Broadcast()
				s.mu.Unlock()
			})
		}
		s.blocked++
		s.space.Wait()
		s.blocked--
	}
}

func (s *scheduler) newKey(key int64) *keyQueue {
	var k *keyQueue
	if n := len(s.free); n > 0 {
		k, s.free = s.free[n-1], s.free[:n-1]
	} else {
		k = &keyQueue{}
	}
	k.key = key
	s.keys[key] = k
	return k
}

// close stops accepting updates; the workers handle what waits and then return.
func (s *scheduler) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.work.Broadcast()
		s.space.Broadcast()
	}
}

// wait blocks until every worker has returned, or until ctx ends.
func (s *scheduler) wait(ctx context.Context) error {
	select {
	case <-s.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ring is a FIFO queue of keys that grows as needed.
type ring struct {
	buf     []*keyQueue
	head, n int
}

func (r *ring) len() int { return r.n }

func (r *ring) push(k *keyQueue) {
	if r.n == len(r.buf) {
		buf := make([]*keyQueue, max(16, 2*len(r.buf)))
		for i := range r.n {
			buf[i] = r.buf[(r.head+i)%len(r.buf)]
		}
		r.buf, r.head = buf, 0
	}
	r.buf[(r.head+r.n)%len(r.buf)] = k
	r.n++
}

func (r *ring) pop() *keyQueue {
	k := r.buf[r.head]
	r.buf[r.head] = nil
	r.head = (r.head + 1) % len(r.buf)
	r.n--
	return k
}
