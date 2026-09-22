// engine/engine.go

package engine

import (
	"context"
	"sync"

	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/clients"
)

type Side int

const (
	BetmaticWin Side = iota
	BetfairBack
	BetfairLay
)

func (s Side) String() string {
	switch s {
	case BetmaticWin:
		return "betmatic-win"
	case BetfairBack:
		return "betfair-back"
	case BetfairLay:
		return "betfair-lay"
	default:
		return "unknown"
	}
}

// Active reports whether anything is staked. MBL counts even with a zero stake
// because it bets the bookmaker's maximum instead of targeting a liability.
func (s Stake) Active() bool { return s.BetsBetmatic() || s.BetsBetfair() }

func (s Stake) BetsBetmatic() bool { return s.Betmatic.WinMBL || s.Betmatic.WinStake > 0 }

func (s Stake) BetsBetfair() bool { return s.Betfair.BackStake > 0 || s.Betfair.LayStake > 0 }

type Engine struct {
	// ctx bounds every session's token refresh; cancelled on runtime stop.
	ctx context.Context

	mu       sync.Mutex
	betmatic map[string]*session[*betmatic.Client]
	betfair  map[string]*session[*betfair.Client]
}

func New(ctx context.Context) *Engine {
	return &Engine{
		ctx:      ctx,
		betmatic: make(map[string]*session[*betmatic.Client]),
		betfair:  make(map[string]*session[*betfair.Client]),
	}
}

// Account opens (or joins) the sessions a process needs and binds them with
// the account fields every bet carries. Sessions are shared by username and
// refcounted by holding process; see account.go.
func (e *Engine) Account(key clients.ProcessKey, c Credentials) (*Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	a := &Account{App: key.App, UserID: key.UserID, ProcessID: key.ProcessID}

	if c.Betmatic != nil {
		client, err := claim(e.ctx, e.betmatic, key, c.Betmatic.Username, func() (*betmatic.Client, error) {
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
		client, err := claim(e.ctx, e.betfair, key, c.Betfair.Username, func() (*betfair.Client, error) {
			return betfair.NewBetfairClient(c.Betfair.Username, c.Betfair.Password, c.Betfair.AppKey, c.Betfair.Cert)
		})
		if err != nil {
			release(e.betmatic, key)
			return nil, err
		}
		a.betfair = client
	}

	return a, nil
}

// Release drops every session hold key has. A session with no holders left is
// closed.
func (e *Engine) Release(key clients.ProcessKey) {
	e.mu.Lock()
	defer e.mu.Unlock()

	release(e.betmatic, key)
	release(e.betfair, key)
}

// Close logs every session out.
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
}
