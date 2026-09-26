// pegasus/app.go
//
// Pegasus: Triple-S race data → per-process strategy → bet. The app owns the
// feed and fans each message out to the processes whose scope it matches.

package pegasus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/dispatch"
	"pegasus_suite/apps/pegasus/process"
	"pegasus_suite/apps/pegasus/settings"
	"pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/clients"
	"pegasus_suite/engine"
	"pegasus_suite/kernel"
	"pegasus_suite/logger"
)

const Name = core.AppName

// FeedStatus is keyed by the feed's settings document name.
type FeedStatus struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type App struct {
	mu sync.RWMutex

	// gen tells a late feed callback from an earlier Start to stand down
	gen     uint64
	eng     *engine.Engine
	triples *triples.Client
	enabled bool

	feed atomic.Pointer[FeedStatus]

	procs map[clients.ProcessKey]*process.Process

	// race keys already reported as reaching no process
	unmatched sync.Map
}

func New() *App {
	return &App{procs: make(map[clients.ProcessKey]*process.Process)}
}

func (a *App) Name() string { return Name }

func (a *App) ProcessSettings() kernel.Settings { return &settings.ProcessSettings{} }

// AdminSettings returns the Triple-S feed document, the only admin document Pegasus owns.
func (a *App) AdminSettings() map[string]func() kernel.Settings {
	return map[string]func() kernel.Settings{
		settings.TripleSDoc: func() kernel.Settings { return settings.DefaultTripleS() },
	}
}

// Start connects Triple-S. A feed that fails to connect leaves the app up and reported down in Status.
func (a *App) Start(ctx context.Context, h kernel.Host) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	doc, err := h.Settings(settings.TripleSDoc)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	cfg, err := settings.ParseTripleS(doc)
	if err != nil {
		return err
	}

	a.gen++
	a.eng = h.Engine()
	a.enabled = cfg.Enabled

	status := FeedStatus{Key: settings.TripleSDoc, Enabled: cfg.Enabled}
	if cfg.Enabled {
		client, err := a.connectTripleS(ctx, *cfg, a.gen)
		if err != nil {
			status.Error = err.Error()
			logger.Error(logger.Log{App: Name, Message: fmt.Sprintf("triple-s did not start; nothing bets until a restart error=%v", err)})
		}
		a.triples = client
	} else {
		logger.Warn(logger.Log{App: Name, Message: "pegasus starting with triple-s disabled; nothing will bet"})
	}
	status.Running = a.triples != nil
	a.feed.Store(&status)
	return nil
}

func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.triples != nil {
		a.triples.Disconnect()
		a.triples = nil
	}
	a.feed.Store(nil)
	a.procs = make(map[clients.ProcessKey]*process.Process)
}

// Status returns the feed's state as a one-element list, the shape the admin page reads.
func (a *App) Status() any {
	if f := a.feed.Load(); f != nil {
		return []FeedStatus{*f}
	}
	return nil
}

func (a *App) connectTripleS(ctx context.Context, s settings.TripleS, gen uint64) (*triples.Client, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return triples.NewClient(ctx, triples.Config{
		Endpoint:        s.Endpoint,
		Region:          s.Region,
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		ClientID:        s.ClientID,
		Topics:          triples.Topics,
	}, a.onTripleS, func(err error) { a.onFeedLost(gen, err) })
}

// onFeedLost disconnects a permanently lost feed and reports it down until a restart.
func (a *App) onFeedLost(gen uint64, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if gen != a.gen {
		return
	}
	if a.triples != nil {
		a.triples.Disconnect()
		a.triples = nil
	}
	if f := a.feed.Load(); f != nil {
		lost := *f
		lost.Running, lost.Error = false, err.Error()
		a.feed.Store(&lost)
	}
	logger.Error(logger.Log{App: Name, Message: fmt.Sprintf("triple-s permanently lost; nothing bets until a restart error=%v", err)})
}

// ---- processes ----

func (a *App) NewProcess(key clients.ProcessKey, doc json.RawMessage, h kernel.Host) (kernel.Process, error) {
	s, err := settings.ParseProcess(key, doc)
	if err != nil {
		return nil, err
	}
	if err := a.validateScopes(s); err != nil {
		return nil, err
	}

	account, err := h.Engine().Account(key, s.Credentials())
	if err != nil {
		return nil, fmt.Errorf("unable to open betting sessions: %w", err)
	}

	d := dispatch.New(h.Engine(), account, a.betfairRace)
	p := process.New(*s, d, a.betfairRace, func() { a.remove(key) })

	a.mu.Lock()
	a.procs[key] = p
	a.mu.Unlock()

	logger.Debug(logger.Log{App: Name, UserID: key.UserID, ProcessID: key.ProcessID, Message: fmt.Sprintf("added process scopes=%v betmatic=%v betfair=%v", activeScopes(s), s.BetmaticCredentials.Username, s.BetfairCredentials.Username)})
	return p, nil
}

func (a *App) remove(key clients.ProcessKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.procs, key)
}

func activeScopes(s *settings.ProcessSettings) []string {
	var keys []string
	for key, scope := range s.Scopes {
		if scope.Active() {
			keys = append(keys, key)
		}
	}
	return keys
}

// validateScopes rejects a process with an active scope outside AU or while Triple-S is disabled.
func (a *App) validateScopes(s *settings.ProcessSettings) error {
	a.mu.RLock()
	enabled := a.enabled
	a.mu.RUnlock()

	for _, key := range activeScopes(s) {
		country, _, ok := core.SplitScopeKey(key)
		if !ok || country != "AU" {
			return fmt.Errorf("unknown scope %q", key)
		}
		if !enabled {
			return fmt.Errorf("scope %s: triple-s is disabled", key)
		}
	}
	return nil
}

// ---- fan-out ----

// onTripleS offers a Triple-S message, with its venue named the Betmatic way, to every process in scope.
func (a *App) onTripleS(m triples.RaceMessage) {
	defer logger.Recover(Name)
	ref, ok := m.Ref()
	if !ok || ref.Scope == "" {
		return
	}
	ref.VenueName = core.CanonicalVenue(ref.Venue, ref.VenueName)

	wanted := false
	a.mu.RLock()
	for _, p := range a.procs {
		if p.OfferTripleS(m, ref) {
			wanted = true
		}
	}
	a.mu.RUnlock()

	if ref.Status == core.StatusFinished {
		a.unmatched.Delete(ref.Key)
		return
	}
	if !wanted {
		a.noMatch(ref)
	}
}

// noMatch logs, once per race, a race that no running process has in scope.
func (a *App) noMatch(ref core.RaceRef) {
	if _, seen := a.unmatched.LoadOrStore(ref.Key, struct{}{}); seen {
		return
	}
	logger.Debug(logger.Log{App: Name, Race: ref.LogRace(), Message: fmt.Sprintf("race reached no process scope=%v status=%v", ref.Scope, ref.Status)})
}

// betfairRace returns the engine's Betfair catalogue entry for ref, or nil.
func (a *App) betfairRace(ref core.RaceRef) *core.BetfairRace {
	a.mu.RLock()
	eng := a.eng
	a.mu.RUnlock()

	track, ok := core.BetfairTrackFor(ref.Venue)
	if eng == nil || !ok {
		return nil
	}
	return eng.BetfairRace(core.BetfairRacingCode(ref.Code), ref.Country, track, ref.RaceNumber)
}

var _ kernel.App = (*App)(nil)
