// logger/sink_ring.go
//
// The live view: the last N entries in memory, read by the /api/logs route.
// Append is one slot write under a mutex, so the emit path pays nothing it
// would notice; reads copy out under the same lock and filter outside it.

package logger

import "sync"

type ringSink struct {
	mu   sync.Mutex
	buf  []Entry
	next int
	full bool
}

func newRingSink(size int) *ringSink {
	return &ringSink{buf: make([]Entry, size)}
}

func (r *ringSink) enqueue(e Entry) {
	r.mu.Lock()
	r.buf[r.next] = e
	r.next++
	if r.next == len(r.buf) {
		r.next = 0
		r.full = true
	}
	r.mu.Unlock()
}

func (r *ringSink) stop() {}

// recent returns matching entries newest first, at most q.Limit (all if 0).
func (r *ringSink) recent(q Query) []Record {
	r.mu.Lock()
	n := r.next
	if r.full {
		n = len(r.buf)
	}
	snapshot := make([]Entry, n)
	if r.full {
		// oldest is at next, so rotate so snapshot is oldest → newest
		copy(snapshot, r.buf[r.next:])
		copy(snapshot[len(r.buf)-r.next:], r.buf[:r.next])
	} else {
		copy(snapshot, r.buf[:r.next])
	}
	r.mu.Unlock()

	out := make([]Record, 0, min(len(snapshot), max(q.Limit, 1)))
	for i := len(snapshot) - 1; i >= 0; i-- {
		if !q.matches(snapshot[i]) {
			continue
		}
		out = append(out, snapshot[i].Record())
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out
}
