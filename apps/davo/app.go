// davo/app.go
//
// DAVO: a tipster's Telegram channel → tips → a Betmatic bet per process.
// Tips can be days ahead; the engine's claims hold them for 96 hours.

package davo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"pegasus_suite/apps/davo/anthropic"
	"pegasus_suite/apps/davo/process"
	"pegasus_suite/apps/davo/settings"
	"pegasus_suite/apps/davo/telegram"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"pegasus_suite/kernel"
	"pegasus_suite/logger"
)

const Name = "davo"

// eventsKey holds the admin Betmatic session that lists upcoming events.
var eventsKey = clients.ProcessKey{App: Name, UserID: "admin", ProcessID: "events"}

type App struct {
	mu       sync.RWMutex
	eng      *engine.Engine
	events   *betmatic.Client
	telegram *telegram.Client
	model    *anthropic.Client // nil without an API key; photos and loose posts are then skipped
	gate     *modelGate
	procs    map[clients.ProcessKey]*process.Process
}

func New() *App {
	return &App{procs: make(map[clients.ProcessKey]*process.Process)}
}

func (a *App) Name() string { return Name }

func (a *App) ProcessSettings() kernel.Settings { return &settings.ProcessSettings{} }

func (a *App) AdminSettings() map[string]func() kernel.Settings {
	return map[string]func() kernel.Settings{
		settings.AdminDoc: func() kernel.Settings { return &settings.Admin{} },
	}
}

// Start opens the admin Betmatic session for the event list, the model client, and the channel poll.
func (a *App) Start(ctx context.Context, h kernel.Host) error {
	doc, err := h.Settings(settings.AdminDoc)
	if err != nil {
		return err
	}
	cfg, err := settings.ParseAdmin(doc)
	if err != nil {
		return err
	}
	if doc, err = h.Settings("betmatic"); err != nil {
		return err
	}
	var admin engine.BetmaticCredentials
	if len(doc) > 0 {
		if err := json.Unmarshal(doc, &admin); err != nil {
			return fmt.Errorf("betmatic admin: %w", err)
		}
	}

	eng := h.Engine()
	events, err := eng.Betmatic(eventsKey, admin)
	if err != nil {
		return fmt.Errorf("betmatic admin: %w", err)
	}
	events.StartUpcomingEventsRefresh(ctx, betmatic.THOROUGHBRED, "AU")

	tg, err := telegram.NewClient(cfg.TelegramBotToken, cfg.ScrapeChannelID)
	if err != nil {
		eng.Release(eventsKey)
		return err
	}

	var model *anthropic.Client
	if cfg.AnthropicAPIKey != "" {
		if model, err = anthropic.NewClient(cfg.AnthropicAPIKey); err != nil {
			eng.Release(eventsKey)
			return err
		}
	} else {
		logger.Warn(logger.Log{App: Name, Message: "no anthropic api key; photo tips and loose posts will be skipped"})
	}

	a.mu.Lock()
	a.eng, a.events, a.telegram, a.model, a.gate = eng, events, tg, model, newModelGate()
	a.mu.Unlock()

	go func() {
		if err := tg.Poll(ctx, a.onPost); err != nil && ctx.Err() == nil {
			logger.Error(logger.Log{App: Name, Message: fmt.Sprintf("telegram polling stopped; no tips until a restart error=%v", err)})
		}
	}()
	return nil
}

func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.eng != nil {
		a.eng.Release(eventsKey)
	}
	a.procs = make(map[clients.ProcessKey]*process.Process)
}

func (a *App) Status() any { return nil }

func (a *App) NewProcess(key clients.ProcessKey, doc json.RawMessage, h kernel.Host) (kernel.Process, error) {
	s, err := settings.ParseProcess(key, doc)
	if err != nil {
		return nil, err
	}
	account, err := h.Engine().Account(key, engine.Credentials{Betmatic: &s.Betmatic})
	if err != nil {
		return nil, fmt.Errorf("unable to open betting sessions: %w", err)
	}

	p := process.New(*s, h.Engine(), account, func() { a.remove(key) })
	a.mu.Lock()
	a.procs[key] = p
	a.mu.Unlock()
	return p, nil
}

func (a *App) remove(key clients.ProcessKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.procs, key)
}

func (a *App) processes() []*process.Process {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*process.Process, 0, len(a.procs))
	for _, p := range a.procs {
		out = append(out, p)
	}
	return out
}

var _ kernel.App = (*App)(nil)
