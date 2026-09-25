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

	mu       sync.Mutex
	betmatic map[string]*session[*betmatic.Client]
	betfair  map[string]*session[*betfair.Client]
	users    map[string]*claims

	// catalogue and live prices; nil until StartBetfair succeeds
	admin *betfair.Client
}

func New(ctx context.Context) *Engine {
	e := &Engine{
		ctx:      ctx,
		betmatic: make(map[string]*session[*betmatic.Client]),
		betfair:  make(map[string]*session[*betfair.Client]),
		users:    make(map[string]*claims),
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

	e.mu.Lock()
	e.admin = admin
	e.mu.Unlock()
	return nil
}

// BetfairRace returns the catalogue's race, or nil when there is none or no admin account.
func (e *Engine) BetfairRace(code betfair.RacingCode, country, track string, number int) *betfair.Race {
	admin := e.adminClient()
	if admin == nil {
		return nil
	}
	return admin.GetRace(code, country, track, number)
}

func (e *Engine) adminClient() *betfair.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.admin
}

// Account opens (or joins) the sessions a process needs and binds them with its user's claims.
func (e *Engine) Account(key clients.ProcessKey, c Credentials) (*Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	a := &Account{App: key.App, UserID: key.UserID, ProcessID: key.ProcessID, claims: e.users[key.UserID]}
	if a.claims == nil {
		a.claims = newClaims()
		e.users[key.UserID] = a.claims
	}

	if c.Betmatic != nil {
		client, err := join(e.ctx, e.betmatic, key, c.Betmatic.Username, func() (*betmatic.Client, error) {
			return betmatic.NewBetmaticClient(c.Betmatic.Username, c.Betmatic.Password)
		})
		if err != nil {
			return nil, err
		}
		a.betmatic = client
		a.BotID = c.Betmatic.BotID
		a.Bookmakers = c.Betmatic.Bookmakers
	}

	if c.Betfair != nil {
		client, err := join(e.ctx, e.betfair, key, c.Betfair.Username, func() (*betfair.Client, error) {
			return betfair.NewBetfairClient(c.Betfair.Username, c.Betfair.Password, c.Betfair.AppKey, c.Betfair.Cert)
		})
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
	if e.admin != nil {
		e.admin.Close()
		e.admin = nil
	}
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
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*claims, 0, len(e.users))
	for _, c := range e.users {
		out = append(out, c)
	}
	return out
}

// logPanic logs a panic instead of letting it crash the binary; call it deferred.
func logPanic() {
	if r := recover(); r != nil {
		logger.Error(logger.Log{Message: fmt.Sprintf("recovered panic: %v", r)})
	}
}
