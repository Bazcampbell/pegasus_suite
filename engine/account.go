// engine/account.go
//
// sharing betting sessions in a pool, avoiding JWT invalidation

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

// Placer is the one method the engine needs from a session. The concrete
// clients satisfy it; tests substitute a recorder.
type Placer interface {
	PlaceBet(betting.BetRequest) error
}

// Account is what a process bets through: its sessions plus the account
// fields every request carries. Built once at process construction; the hot
// path reads it and never looks anything up.
type Account struct {
	App       string
	UserID    string
	ProcessID string

	BotID      string
	Bookmakers []string

	// nil when the process has no account with that provider
	betmatic Placer
	betfair  Placer
}

func (a *Account) HasBetmatic() bool { return a != nil && a.betmatic != nil }
func (a *Account) HasBetfair() bool  { return a != nil && a.betfair != nil }

// TestAccount binds arbitrary placers, for application tests that need the
// decision path without a bookmaker. Nil placers are allowed.
func TestAccount(userID, processID string, bm, bf Placer) *Account {
	a := &Account{UserID: userID, ProcessID: processID}
	if bm != nil {
		a.betmatic = bm
	}
	if bf != nil {
		a.betfair = bf
	}
	return a
}

type session[C betting.Client] struct {
	client  C
	holders map[clients.ProcessKey]struct{}
}

func claim[C betting.Client](ctx context.Context, m map[string]*session[C], key clients.ProcessKey, username string, dial func() (C, error)) (C, error) {
	alias := normalise(username)

	s, ok := m[alias]
	if !ok {
		client, err := dial()
		if err != nil {
			var zero C
			return zero, err
		}
		// StartTokenRefresh replaces the client's cancel func, so it runs once
		// per session, here, and never again on a re-claim.
		client.StartTokenRefresh(ctx)

		s = &session[C]{client: client, holders: make(map[clients.ProcessKey]struct{})}
		m[alias] = s

		logger.Debug(logger.Log{
			Application:      key.App,
			FormattedMessage: "opened " + string(client.Provider()) + " session account=" + username,
			UserID:           key.UserID,
			ProcessID:        key.ProcessID,
		})
	}

	s.holders[key] = struct{}{}
	return s.client, nil
}

func release[C betting.Client](m map[string]*session[C], key clients.ProcessKey) {
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

		logger.Debug(logger.Log{
			Application:      key.App,
			FormattedMessage: "closed " + string(s.client.Provider()) + " session account=" + alias,
			UserID:           key.UserID,
			ProcessID:        key.ProcessID,
		})
	}
}

func normalise(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
