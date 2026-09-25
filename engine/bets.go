// engine/bets.go
//
// Reading back what was bet, through the same pooled sessions the bets went
// out on, so a report never logs a live session out.

package engine

import (
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/clients"
)

// BetmaticBets returns every notification on account c for a meeting date (YYYY-MM-DD).
func (e *Engine) BetmaticBets(c BetmaticCredentials, date string) ([]betting.Bet, error) {
	key := readerKey(c.Username)
	e.mu.Lock()
	client, err := e.joinBetmatic(key, c)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer e.Release(key)
	return client.MeetingBets(date)
}

// BetfairBets returns every bet on account c cleared in [from, to).
func (e *Engine) BetfairBets(c BetfairCredentials, from, to time.Time) ([]betting.Bet, error) {
	key := readerKey(c.Username)
	e.mu.Lock()
	client, err := e.joinBetfair(key, c)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer e.Release(key)
	return client.SettledBets(from, to)
}

func readerKey(username string) clients.ProcessKey {
	return clients.ProcessKey{App: "report", UserID: username, ProcessID: "report"}
}
