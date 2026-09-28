package bot

import (
	"testing"

	"github.com/DhurghamAhmed/teleiq/models"
)

func TestTracker(t *testing.T) {
	type step struct {
		add, handled int64 // one of them
		freeze       bool
		resync       bool
		wantAdded    bool
		wantOffset   int64
		wantOpen     bool
		wantMoved    bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"nothing yet", []step{{wantOffset: 0}}},
		{"one update", []step{
			{add: 5, wantAdded: true, wantOffset: 5, wantOpen: true},
			{handled: 5, wantOffset: 6, wantMoved: true},
		}},
		{"handled out of order", []step{
			{add: 5, wantAdded: true, wantOffset: 5, wantOpen: true},
			{add: 6, wantAdded: true, wantOffset: 5, wantOpen: true},
			{add: 8, wantAdded: true, wantOffset: 5, wantOpen: true},
			{handled: 6, wantOffset: 5, wantOpen: true},
			{handled: 5, wantOffset: 8, wantOpen: true, wantMoved: true},
			{handled: 8, wantOffset: 9, wantMoved: true},
		}},
		{"seen again", []step{
			{add: 5, wantAdded: true, wantOffset: 5, wantOpen: true},
			{add: 5, wantOffset: 5, wantOpen: true},
			{add: 4, wantOffset: 5, wantOpen: true},
			{handled: 5, wantOffset: 6, wantMoved: true},
			{add: 5, wantOffset: 6},
		}},
		{"frozen", []step{
			{add: 5, wantAdded: true, wantOffset: 5, wantOpen: true},
			{freeze: true, wantOffset: 5, wantOpen: true},
			{handled: 5, wantOffset: 5, wantOpen: true},
		}},
		{"resync", []step{
			{add: 9, wantAdded: true, wantOffset: 9, wantOpen: true},
			{handled: 9, wantOffset: 10, wantMoved: true},
			{resync: true, wantOffset: 0},
			{add: 2, wantAdded: true, wantOffset: 2, wantOpen: true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTracker()
			for i, s := range tt.steps {
				added := false
				switch {
				case s.add != 0:
					added = tr.add(s.add)
				case s.handled != 0:
					tr.handled(&models.Update{UpdateID: s.handled})
				case s.freeze:
					tr.freeze()
				case s.resync:
					tr.resync()
				}
				moved := false
				select {
				case <-tr.moved:
					moved = true
				default:
				}
				offset, open := tr.offset()
				if added != s.wantAdded || offset != s.wantOffset || open != s.wantOpen || moved != s.wantMoved {
					t.Errorf("step %d %+v: added %v, offset %d, open %v, moved %v", i, s, added, offset, open, moved)
				}
			}
		})
	}
}

// TestTrackerReusesItsArray: handing over and handling updates for long does not grow the array,
// even when the front never catches up with the end.
func TestTrackerReusesItsArray(t *testing.T) {
	tr := newTracker()
	id := int64(0)
	for range 10_000 {
		for range 4 {
			id++
			tr.add(id)
		}
		// The newest update stays open until the next round.
		for i := max(id-4, 1); i < id; i++ {
			tr.handled(&models.Update{UpdateID: i})
		}
	}
	if offset, open := tr.offset(); offset != id || !open {
		t.Fatalf("offset %d (open %v), want %d, open", offset, open, id)
	}
	if c := cap(tr.open); c > 16 {
		t.Errorf("array of %d updates after %d, want it reused", c, id)
	}
}
