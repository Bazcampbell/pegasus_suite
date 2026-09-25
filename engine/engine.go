// engine/engine.go
//
// One engine per runtime, shared by every process of every app. It owns the
// bookmaker sessions, the admin Betfair catalogue and price stream, dedupe,
// and placement.

package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/clients"
	"pegasus_suite/logger"
)

var betfairCountries = []string{"AU"}

type Engine struct {
	// bounds every background loop; cancelled on runtime stop
	ctx context.Context

	// guards the session pools; never taken on the bet path
	mu       sync.Mutex
	betmatic map[string]*session[*betmatic.Client]
	betfair  map[string]*session[*betfair.Client]

	users sync.Map // user ID → *claims

	// catalogue and live prices; nil until StartBetfair succeeds
	admin atomic.Pointer[betfair.Client]
}

func New(ctx context.Context) *Engine {
	e := &Engine{
		ctx:      ctx,
		betmatic: make(map[string]*session[*betmatic.Client]),
		betfair:  make(map[string]*session[*betfair.Client]),
	}
	go e.expireClaims()
	return e
}

// StartBetfair logs the admin account in and starts the race catalogue and the price stream.
func (e *Engine) StartBetfair(c BetfairCredentials) error {
	if err := c.Validate(); err != nil {
		return err
	}
	admin, err := betfair.NewBetfairClient(c.Username, c.Password, c.AppKey, c.Cert)
	if err != nil {
		return fmt.Errorf("betfair admin: %w", err)
	}
	admin.StartTokenRefresh(e.ctx)
	admin.StartTrackRefresh(e.ctx, betfairCountries)
	admin.StartRunnerUpdates(e.ctx, e.closeMarket)

	e.admin.Store(admin)
	return nil
}

// BetfairRace returns the catalogue's race, or nil when there is none or no admin account.
func (e *Engine) BetfairRace(code betfair.RacingCode, country, track string, number int) *betfair.Race {
	admin := e.admin.Load()
	if admin == nil {
		return nil
	}
	return admin.GetRace(code, country, track, number)
}

// Account opens (or joins) the sessions a process needs and binds them with its user's claims.
func (e *Engine) Account(key clients.ProcessKey, creds Credentials) (*Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, _ := e.users.LoadOrStore(key.UserID, newClaims())
	a := &Account{App: key.App, UserID: key.UserID, ProcessID: key.ProcessID, claims: c.(*claims)}

	if creds.Betmatic != nil {
		client, err := e.joinBetmatic(key, *creds.Betmatic)
		if err != nil {
			return nil, err
		}
		a.betmatic = client
		a.BotID = creds.Betmatic.BotID
		a.Bookmakers = creds.Betmatic.Bookmakers
	}

	if creds.Betfair != nil {
		client, err := e.joinBetfair(key, *creds.Betfair)
		if err != nil {
			leave(e.betmatic, key)
			return nil, err
		}
		a.betfair = client
	}

	return a, nil
}

// Release drops every session hold key has, closing sessions nobody holds.
func (e *Engine) Release(key clients.ProcessKey) {
	e.mu.Lock()
	defer e.mu.Unlock()

	leave(e.betmatic, key)
	leave(e.betfair, key)
}

// Close logs every session out, the admin account included.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for alias, s := range e.betmatic {
		s.client.Close()
		delete(e.betmatic, alias)
	}
	for alias, s := range e.betfair {
		s.client.Close()
		delete(e.betfair, alias)
	}
	if admin := e.admin.Swap(nil); admin != nil {
		admin.Close()
	}
}

// joinBetmatic returns the pooled Betmatic session for c, held by key. Callers hold e.mu.
func (e *Engine) joinBetmatic(key clients.ProcessKey, c BetmaticCredentials) (*betmatic.Client, error) {
	return join(e.ctx, e.betmatic, key, c.Username, func() (*betmatic.Client, error) {
		return betmatic.NewBetmaticClient(c.Username, c.Password)
	})
}

// joinBetfair returns the pooled Betfair session for c, held by key. Callers hold e.mu.
func (e *Engine) joinBetfair(key clients.ProcessKey, c BetfairCredentials) (*betfair.Client, error) {
	return join(e.ctx, e.betfair, key, c.Username, func() (*betfair.Client, error) {
		return betfair.NewBetfairClient(c.Username, c.Password, c.AppKey, c.Cert)
	})
}

// closeMarket frees every claim on a market the stream reports closed.
func (e *Engine) closeMarket(marketID string) {
	for _, c := range e.allClaims() {
		c.drop(marketID, time.Time{})
	}
}

// expireClaims drops claims older than claimTTL every hour until the engine's context ends.
func (e *Engine) expireClaims() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for _, c := range e.allClaims() {
				c.drop("", time.Now().Add(-claimTTL))
			}
		case <-e.ctx.Done():
			return
		}
	}
}

func (e *Engine) allClaims() []*claims {
	var out []*claims
	e.users.Range(func(_, c any) bool {
		out = append(out, c.(*claims))
		return true
	})
	return out
}
