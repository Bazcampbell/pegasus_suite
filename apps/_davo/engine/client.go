// engine/client.go

package engine

import (
	"racing_wagering/apps/davo/anthropic"
	"racing_wagering/apps/davo/core"
	"racing_wagering/apps/davo/store"
	"racing_wagering/apps/davo/telegram"
	"racing_wagering/apps/davo/tenant"
	"sync"

	"racing_wagering/betting/betmatic"
	logger "racing_wagering/logger"
)

// poll telegram > parse msg > fan to processes

type Engine struct {
	db *store.DB
	mu sync.RWMutex

	adminClient *betmatic.Client

	telegram *telegram.Client

	anthropic *anthropic.Client

	// Decides what is worth spending a model call on: dedupe, rate limit, and the
	// structural checks that keep non-tips away from the API.
	gate *visionGate

	processes map[string]map[string]*tenant.Process // userID:process ID:process
}

func NewEngine(db *store.DB, adminClient *betmatic.Client, tc *telegram.Client, vc *anthropic.Client) (*Engine, error) {
	return &Engine{
		db:          db,
		adminClient: adminClient,
		telegram:    tc,
		anthropic:   vc,
		gate:        newVisionGate(),
		processes:   make(map[string]map[string]*tenant.Process),
	}, nil
}

type ProcessKey struct {
	UserID    string
	ProcessID string
	Running   bool
}

func (e *Engine) ProcessKeys() []ProcessKey {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var keys []ProcessKey
	for userID, byID := range e.processes {
		for processID, p := range byID {
			keys = append(keys, ProcessKey{UserID: userID, ProcessID: processID, Running: p.Status()})
		}
	}
	return keys
}

func (e *Engine) StopAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, byID := range e.processes {
		for _, p := range byID {
			p.Stop()
		}
	}
}

// sends pre-bet warning to processes
func (e *Engine) processPreBetMessage() {
	e.mu.RLock()
	running := make([]*tenant.Process, 0)
	for userID := range e.processes {
		for _, process := range e.processes[userID] {
			if process.Status() {
				running = append(running, process)
			}
		}
	}
	e.mu.RUnlock()

	for _, process := range running {
		go process.OnPreBetMessage()
	}
}

func (e *Engine) processbetMessage(m core.DavoRaceMessage) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for userID := range e.processes {
		for _, process := range e.processes[userID] {
			if !process.Status() {
				continue
			}

			select {
			case process.RaceDataInbox <- m:

			default:
				logger.Warn(logger.ErrorLog{
					Message:   "race inbox full",
					UserID:    process.Settings.UserID,
					ProcessID: process.Settings.ID,
					Request:   m,
					RaceDetails: &logger.RaceDetails{
						Venue:      m.Venue,
						RaceNumber: m.RaceNumber,
					},
				})
			}
		}
	}
}
