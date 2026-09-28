package bot

import (
	"sync"

	"github.com/DhurghamAhmed/teleiq/models"
)

// tracker keeps the updates handed over that the offset cannot confirm yet.
type tracker struct {
	mu     sync.Mutex
	open   []openUpdate // open[head:] in the order they were handed over, which is their order
	head   int
	seen   int64         // the highest update_id handed over; 0 before any and after a resync
	frozen bool          // handled updates are no longer recorded
	moved  chan struct{} // receives when the oldest open update has been handled
}

type openUpdate struct {
	id      int64
	handled bool
}

func newTracker() *tracker {
	return &tracker{moved: make(chan struct{}, 1)}
}

// add records the update id as handed over, reporting false if seen before.
func (t *tracker) add(id int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id <= t.seen {
		return false
	}
	t.seen = id
	if t.head > 0 && len(t.open) == cap(t.open) {
		n := copy(t.open, t.open[t.head:])
		t.open, t.head = t.open[:n], 0
	}
	t.open = append(t.open, openUpdate{id: id})
	return true
}

// handled records that u has been handled and drops handled updates at the front.
func (t *tracker) handled(u *models.Update) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.frozen {
		return
	}
	for i := t.head; i < len(t.open); i++ {
		if t.open[i].id == u.UpdateID {
			t.open[i].handled = true
			break
		}
	}
	n := t.head
	for n < len(t.open) && t.open[n].handled {
		n++
	}
	if n == t.head {
		return
	}
	t.head = n
	if t.head == len(t.open) {
		t.open, t.head = t.open[:0], 0
	}
	select {
	case t.moved <- struct{}{}:
	default:
	}
}

// offset returns the offset confirming handled updates and whether one is open.
func (t *tracker) offset() (offset int64, open bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.head < len(t.open):
		return t.open[t.head].id, true
	case t.seen == 0:
		return 0, false
	}
	return t.seen + 1, false
}

// freeze stops recording handled updates, so canceled handlers do not count.
func (t *tracker) freeze() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frozen = true
}

// resync forgets the updates seen, after a getUpdates without an offset.
func (t *tracker) resync() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen = 0
}
