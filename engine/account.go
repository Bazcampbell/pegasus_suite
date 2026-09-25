// engine/account.go
//
// Sessions are pooled by username and refcounted by process, so one login
// serves every process on the same account.

package engine

import (
	"context"
	"errors"
	"strings"

	"pegasus_suite/betting"
	"pegasus_suite/clients"
	"pegasus_suite/logger"
)

type BetmaticCredentials struct {
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	BotID      string   `json:"bot_id"`
	Bookmakers []string `json:"bookmakers"`
}

type BetfairCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	AppKey   string `json:"app_key"`
	Cert     string `json:"cert"`
}

func (c BetmaticCredentials) Validate() error {
	if c.Username == "" || c.Password == "" {
		return errors.New("betmatic username and password required")
	}
	return nil
}

func (c BetfairCredentials) Validate() error {
	if c.Username == "" || c.Password == "" || c.AppKey == "" || c.Cert == "" {
		return errors.New("betfair username, password, app key and cert required")
	}
	return nil
}

// Credentials names the accounts a process bets with. Nil means none.
type Credentials struct {
	Betmatic *BetmaticCredentials
	Betfair  *BetfairCredentials
}

// Placer is the one method the engine needs from a session; tests substitute a recorder.
type Placer interface {
	PlaceBet(betting.BetRequest) (string, error)
}

// Account is what a process bets through: its sessions, the account fields every request
// carries, and its user's claims. Built once per process; the bet path never looks anything up.
type Account struct {
	App       string
	UserID    string
	ProcessID string

	BotID      string
	Bookmakers []string

	// nil when the process has no account with that provider
	betmatic Placer
	betfair  Placer

	claims *claims
}

// TestAccount binds arbitrary placers, either of which may be nil, for tests without a bookmaker.
func TestAccount(app, userID, processID string, bm, bf Placer) *Account {
	return &Account{App: app, UserID: userID, ProcessID: processID, betmatic: bm, betfair: bf, claims: newClaims()}
}

type session[C betting.Client] struct {
	client  C
	holders map[clients.ProcessKey]struct{}
}

// join returns the session for username, dialling it on first use, and records key as a holder.
func join[C betting.Client](ctx context.Context, m map[string]*session[C], key clients.ProcessKey, username string, dial func() (C, error)) (C, error) {
	alias := normalise(username)

	s, ok := m[alias]
	if !ok {
		client, err := dial()
		if err != nil {
			var zero C
			return zero, err
		}
		client.StartTokenRefresh(ctx)

		s = &session[C]{client: client, holders: make(map[clients.ProcessKey]struct{})}
		m[alias] = s

		logger.Debug(logger.Log{App: key.App, UserID: key.UserID, ProcessID: key.ProcessID, Message: "opened " + string(client.Provider()) + " session account=" + username})
	}

	s.holders[key] = struct{}{}
	return s.client, nil
}

// leave drops key from every session it holds, closing sessions left with no holder.
func leave[C betting.Client](m map[string]*session[C], key clients.ProcessKey) {
	for alias, s := range m {
		if _, held := s.holders[key]; !held {
			continue
		}
		delete(s.holders, key)
		if len(s.holders) > 0 {
			continue
		}
		delete(m, alias)
		s.client.Close()

		logger.Debug(logger.Log{App: key.App, UserID: key.UserID, ProcessID: key.ProcessID, Message: "closed " + string(s.client.Provider()) + " session account=" + alias})
	}
}

func normalise(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
