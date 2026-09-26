// engine/bets.go
//
// Reading back what was bet, through the same pooled sessions the bets went
// out on, so a report never logs a live session out.

package engine

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/clients"
)

// BetmaticBets returns every notification on account c for a meeting date (YYYY-MM-DD).
func (e *Engine) BetmaticBets(c BetmaticCredentials, date string) ([]betting.Bet, error) {
	key := readerKey(c.Username)
	client, err := e.Betmatic(key, c)
	if err != nil {
		return nil, err
	}
	defer e.Release(key)
	return client.MeetingBets(date)
}

// Betmatic returns the pooled Betmatic session for c, held by key until Release(key).
func (e *Engine) Betmatic(key clients.ProcessKey, c BetmaticCredentials) (*betmatic.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.joinBetmatic(key, c)
}

// ArmBookies turns on the account's bookmakers for its bot so a bet can fill at once.
func (e *Engine) ArmBookies(a *Account) error {
	client, ok := a.betmatic.(*betmatic.Client)
	if !ok {
		return errors.New("process has no betmatic session")
	}
	ids := make([]int, 0, len(a.Bookmakers))
	for _, b := range a.Bookmakers {
		id, err := strconv.Atoi(b)
		if err != nil {
			return fmt.Errorf("bookmaker %q is not an id", b)
		}
		ids = append(ids, id)
	}
	return errors.Join(client.TurnOnBookies(ids, a.BotID)...)
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
