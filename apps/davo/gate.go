// davo/gate.go

package davo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"pegasus_suite/apps/davo/telegram"
)

const (
	maxTipTextLen = 500

	dedupeTTL = 10 * time.Minute

	maxCallsPerWindow = 300
	rateWindow        = time.Hour
)

// modelGate decides which messages are worth a model call: no repeats, and a rate limit.
type modelGate struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	calls []time.Time
}

func newModelGate() *modelGate {
	return &modelGate{seen: make(map[string]time.Time)}
}

// photoUsable reports whether a message carries a photo worth reading, and why not when it does not.
func photoUsable(msg telegram.Message) (bool, string) {
	if msg.PhotoFileID == "" {
		return false, "no photo"
	}
	if len(msg.Text) > maxTipTextLen {
		return false, fmt.Sprintf("caption too long (%d chars)", len(msg.Text))
	}
	return true, ""
}

// admit reports whether a call keyed by key may go ahead, recording it when it may.
func (g *modelGate) admit(key string) (bool, string) {
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	g.prune(now)

	if seenAt, ok := g.seen[key]; ok {
		return false, fmt.Sprintf("duplicate of a message seen %s ago", now.Sub(seenAt).Round(time.Second))
	}
	if len(g.calls) >= maxCallsPerWindow {
		return false, fmt.Sprintf("rate limit: %d calls in the last %s", len(g.calls), rateWindow)
	}

	g.seen[key] = now
	g.calls = append(g.calls, now)
	return true, ""
}

// prune drops expired repeats and calls that have left the rate window.
func (g *modelGate) prune(now time.Time) {
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

func photoKey(fileID string) string { return "photo:" + fileID }

func tipKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "tip:" + hex.EncodeToString(sum[:8])
}
