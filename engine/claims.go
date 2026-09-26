// engine/claims.go
//
// Dedupe. A user's runner belongs to the first app that bets it: any process
// of that app may bet it once per provider, and every other app is refused.
// Once a bet is sent its claim stands, accepted or rejected.

package engine

import (
	"sync"
	"time"

	"pegasus_suite/betting"
)

// claimTTL outlives the furthest-ahead tip; claims on a streamed market go sooner, when it closes.
const claimTTL = 96 * time.Hour

type claims struct {
	mu sync.Mutex
	m  map[string]*claim // bet ID → claim
}

type claim struct {
	app    string
	market string
	at     time.Time
	legs   map[leg]bool
}

type leg struct {
	process  string
	provider betting.Provider
}

func newClaims() *claims { return &claims{m: make(map[string]*claim)} }

// take records a's bet on id with provider and reports true, or reports false when the bet is a duplicate.
func (c *claims) take(a *Account, id, market string, provider betting.Provider) bool {
	l := leg{process: a.ProcessID, provider: provider}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, ok := c.m[id]
	if !ok {
		c.m[id] = &claim{app: a.App, market: market, at: time.Now(), legs: map[leg]bool{l: true}}
		return true
	}
	if cl.app != a.App || cl.legs[l] {
		return false
	}
	cl.legs[l] = true
	return true
}

// release undoes a take whose bet was never sent, freeing the runner when no other leg holds it.
func (c *claims) release(a *Account, id string, provider betting.Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cl, ok := c.m[id]
	if !ok {
		return
	}
	delete(cl.legs, leg{process: a.ProcessID, provider: provider})
	if len(cl.legs) == 0 {
		delete(c.m, id)
	}
}

// drop removes every claim on market, and every claim taken before cutoff.
func (c *claims) drop(market string, cutoff time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, cl := range c.m {
		if (market != "" && cl.market == market) || cl.at.Before(cutoff) {
			delete(c.m, id)
		}
	}
}
