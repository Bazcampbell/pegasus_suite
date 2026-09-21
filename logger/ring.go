// ring.go
//
// last N records in memory, served to /API/logs
// write is one slot under mutex, read copies out under the same lock, filters outside

package logger

import (
	"strings"
	"sync"
	"time"
)

type ring struct {
	mu   sync.Mutex
	buf  []Record
	next int
	full bool
}

func newRing(size int) *ring { return &ring{buf: make([]Record, size)} }

func (r *ring) add(rec Record) {
	r.mu.Lock()
	r.buf[r.next] = rec
	r.next++
	if r.next == len(r.buf) {
		r.next, r.full = 0, true
	}
	r.mu.Unlock()
}

// returns matching records, newest first
func (r *ring) recent(q Query) []Record {
	r.mu.Lock()
	n := r.next
	if r.full {
		n = len(r.buf)
	}
	snapshot := make([]Record, n)
	if r.full {
		// oldest is at next: rotate so the snapshot runs oldest → newest
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
		out = append(out, snapshot[i])
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out
}

type Query struct {
	Level       string // DEBUG, INFO, WARN, ERROR
	Application string
	UserID      string
	ProcessID   string
	Since       time.Time
	Limit       int
}

func (q Query) matches(r Record) bool {
	switch {
	case q.Level != "" && !strings.EqualFold(q.Level, r.Level),
		q.Application != "" && q.Application != r.Application,
		q.UserID != "" && q.UserID != r.UserID,
		q.ProcessID != "" && q.ProcessID != r.ProcessID,
		!q.Since.IsZero() && r.Time.Before(q.Since):
		return false
	}
	return true
}
