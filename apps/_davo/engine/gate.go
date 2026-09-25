// engine/gate.go

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"pegasus_suite/apps/davo/telegram"
	logger "pegasus_suite/logger"
	"sync"
	"time"
)

const (
	maxTipTextLen = 500

	dedupeTTL = 10 * time.Minute

	maxCallsPerWindow = 300
	rateWindow        = time.Hour
)

// stops definite non-bets from wasting API calls
type visionGate struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	calls []time.Time
	now   func() time.Time
}

func newVisionGate() *visionGate {
	return &visionGate{
		seen: make(map[string]time.Time),
		now:  time.Now,
	}
}

func photoUsable(msg telegram.Message) (bool, string) {
	if msg.PhotoFileID == "" {
		return false, "no photo"
	}

	if len(msg.Text) > maxTipTextLen {
		return false, fmt.Sprintf("caption too long (%d chars)", len(msg.Text))
	}

	return true, ""
}

// reports whether a call identified by key should go ahead, and records it
// when it does. Dedupe and the rate limit are checked together so two posts
// arriving at once cannot both slip past the budget.
func (g *visionGate) admit(key string) (bool, string) {
	now := g.now()

	g.mu.Lock()
	defer g.mu.Unlock()

	g.prune(now)

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("gate: checking key=%v calls_in_window=%v limit=%v", key, len(g.calls), maxCallsPerWindow),
	})

	if seenAt, ok := g.seen[key]; ok {
		return false, fmt.Sprintf("duplicate of a message seen %s ago", now.Sub(seenAt).Round(time.Second))
	}

	if len(g.calls) >= maxCallsPerWindow {
		oldest := g.calls[0]
		return false, fmt.Sprintf("rate limit: %d calls in the last %s, next slot in %s",
			len(g.calls), rateWindow, (oldest.Add(rateWindow).Sub(now)).Round(time.Second))
	}

	g.seen[key] = now
	g.calls = append(g.calls, now)

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("gate: admitted key=%v calls_in_window=%v", key, len(g.calls)),
	})
	return true, ""
}

// prune drops expired dedupe entries and calls that have aged out of rate window
func (g *visionGate) prune(now time.Time) {
	for k, at := range g.seen {
		if now.Sub(at) > dedupeTTL {
			delete(g.seen, k)
		}
	}

	cutoff := now.Add(-rateWindow)
	kept := g.calls[:0]
	for _, at := range g.calls {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	g.calls = kept
}

func photoKey(fileID string) string {
	return "photo:" + fileID
}

func tipKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "tip:" + hex.EncodeToString(sum[:8])
}
