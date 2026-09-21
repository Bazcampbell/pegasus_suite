// pegasus/app.go
//
// Pegasus: live race data → per-process strategy → bet. The application owns
// its feeds (Triple-S over MQTT, TPD over UDP), the admin Betfair client used
// for race and price lookup, and the fan-out of race updates to its processes.

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
	"pegasus_suite/apps/pegasus/strategy"
	"pegasus_suite/apps/pegasus/tpd"
	"pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/betting/betfair"
	"pegasus_suite/clients"
	"pegasus_suite/kernel"
	"pegasus_suite/logger"
)

const Name = core.AppName

// Feed identifiers. The key is the feed's settings document name, so the
// Program tab joins FeedStatus to the toggle that edits that document.
const (
	feedTripleS = "triple-s"
	feedTPD     = "tpd"

	keyTripleS = settings.TripleSDoc
	keyTPD     = settings.TPDDoc
)

// FeedStatus is keyed by the feed's settings document name.
type FeedStatus struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type App struct {
	mu sync.RWMutex

	// per-Start state; gen tells a late feed callback from a previous
	// generation to stand down
	gen     uint64
	ctx     context.Context
	betfair *betfair.Client
	triples *triples.Client
	tpd     *tpd.Client
	enabled map[core.Provider]bool

	feeds atomic.Pointer[[]FeedStatus]

	procs map[clients.ProcessKey]*process.Process

	// Race keys already reported as matching no process, so the report is once
	// per race rather than on every tick.
	unmatched      sync.Map
	betfairMissing sync.Map
}

func New() *App {
	return &App{procs: make(map[clients.ProcessKey]*process.Process)}
}

func (a *App) Name() string { return Name }

func (a *App) ProcessSettings() kernel.Settings { return &settings.ProcessSettings{} }

// AdminSettings are the feed documents Pegasus owns. The admin Betfair account
// it also reads is shared, so the kernel owns that one.
func (a *App) AdminSettings() map[string]func() kernel.Settings {
	return map[string]func() kernel.Settings{
		settings.TripleSDoc: func() kernel.Settings { return settings.DefaultTripleS() },
		settings.TPDDoc:     func() kernel.Settings { return settings.DefaultTPD() },
	}
}

func (a *App) Start(ctx context.Context, h kernel.Host) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.gen++
	gen := a.gen

	doc, err := h.Settings(settings.TripleSDoc)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	tripleSCfg, err := settings.ParseTripleS(doc)
	if err != nil {
		return err
	}
	if doc, err = h.Settings(settings.TPDDoc); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	tpdCfg, err := settings.ParseTPD(doc)
	if err != nil {
		return err
	}

	logger.Info(logger.Log{Application: core.AppName, FormattedMessage: fmt.Sprintf("pegasus starting triple_s=%v tpd=%v", tripleSCfg.Enabled, tpdCfg.Enabled)})

	if !tripleSCfg.Enabled && !tpdCfg.Enabled {
		logger.Warn(logger.Log{Application: core.AppName, FormattedMessage: "pegasus starting with every live feed disabled; no race data will arrive and nothing will bet"})
	}

	// The admin exchange account resolves races and polls prices for every
	// process. Without it nothing can be priced, so it is the one hard failure.
	bf, err := a.setupBetfair(ctx, h)
	if err != nil {
		return err
	}

	a.ctx = ctx
	a.betfair = bf
	a.enabled = map[core.Provider]bool{
		core.ProviderTripleS: tripleSCfg.Enabled,
		core.ProviderTPD:     tpdCfg.Enabled,
	}

	// Neither feed is fatal: one being unreachable is not a reason to deny the
	// other. The app comes up without it, Status says which one is down and
	// why, and a restart brings it back once the source is up.
	tripleS := FeedStatus{Key: keyTripleS, Enabled: tripleSCfg.Enabled}
	if tripleSCfg.Enabled {
		client, err := a.setupTriples(ctx, *tripleSCfg, gen)
		if err != nil {
			tripleS.Error = err.Error()
			logger.Error(logger.Log{Application: core.AppName, FormattedMessage: fmt.Sprintf("triple-s did not start; nothing will bet off it until a restart error=%v", err)})
		}
		a.triples = client
	}
	tripleS.Running = a.triples != nil

	tpdStatus := FeedStatus{Key: keyTPD, Enabled: tpdCfg.Enabled}
	if tpdCfg.Enabled {
		client, err := a.setupTPD(ctx, *tpdCfg, gen)
		if err != nil {
			tpdStatus.Error = err.Error()
			logger.Error(logger.Log{Application: core.AppName, FormattedMessage: fmt.Sprintf("tpd did not start; nothing will bet off it until a restart error=%v", err)})
		}
		a.tpd = client
	}
	tpdStatus.Running = a.tpd != nil

	a.feeds.Store(&[]FeedStatus{tripleS, tpdStatus})
	return nil
}

func (a *App) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.triples != nil {
		a.triples.Disconnect()
		a.triples = nil
	}
	if a.tpd != nil {
		a.tpd.Close()
		a.tpd = nil
	}
	if a.betfair != nil {
		a.betfair.Close()
		a.betfair = nil
	}
	a.feeds.Store(nil)
	a.procs = make(map[clients.ProcessKey]*process.Process)
}

func (a *App) Status() any {
	if p := a.feeds.Load(); p != nil {
		return *p
	}
	return nil
}

// ---- setup ----

func (a *App) setupBetfair(ctx context.Context, h kernel.Host) (*betfair.Client, error) {
	doc, err := h.Settings("betfair")
	if err != nil {
		return nil, err
	}
	creds, err := settings.ParseAdminBetfair(doc)
	if err != nil {
		return nil, err
	}

	client, err := betfair.NewBetfairClient(creds.Username, creds.Password, creds.AppKey, creds.Cert)
	if err != nil {
		return nil, fmt.Errorf("betfair admin: %w", err)
	}

	client.StartTokenRefresh(ctx)
	client.StartTrackRefresh(ctx, core.ScopeCountries)

	logger.Info(logger.Log{Application: core.AppName, FormattedMessage: fmt.Sprintf("betfair track refresh started countries=%v", core.ScopeCountries)})
	return client, nil
}

func (a *App) setupTriples(ctx context.Context, s settings.TripleS, gen uint64) (*triples.Client, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	cfg := triples.Config{
		Endpoint:        s.Endpoint,
		Region:          s.Region,
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		ClientID:        s.ClientID,
		Topics:          triples.Topics,
	}

	return triples.NewClient(ctx, cfg, a.onTripleS, func(e error) {
		a.onFeedFatal(gen, feedTripleS, e)
	})
}

func (a *App) setupTPD(ctx context.Context, s settings.TPD, gen uint64) (*tpd.Client, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return tpd.NewClient(ctx, s.UDPPort, s.LicenceKey, a.onTPD, func(e error) {
		a.onFeedFatal(gen, feedTPD, e)
	})
}

// onFeedFatal takes one permanently lost feed out of service and leaves the
// rest betting: its client is closed and Status reports it down with the
// reason. A restart brings it back.
func (a *App) onFeedFatal(gen uint64, feed string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if gen != a.gen {
		return
	}

	var key string
	switch feed {
	case feedTripleS:
		key = keyTripleS
		if a.triples != nil {
			a.triples.Disconnect()
			a.triples = nil
		}
	case feedTPD:
		key = keyTPD
		if a.tpd != nil {
			a.tpd.Close()
			a.tpd = nil
		}
	}

	// Copied rather than mutated in place so a concurrent Status cannot read a
	// half-updated slice.
	if current := a.feeds.Load(); current != nil {
		updated := make([]FeedStatus, len(*current))
		copy(updated, *current)
		for i := range updated {
			if updated[i].Key == key {
				updated[i].Running = false
				updated[i].Error = err.Error()
			}
		}
		a.feeds.Store(&updated)
	}

	logger.Error(logger.Log{Application: core.AppName, FormattedMessage: fmt.Sprintf("%s permanently lost; it will not bet again until a restart error=%v", feed, err)})
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

	// Sessions are opened once here; the bet path never looks them up again.
	account, err := h.Engine().Account(key, s.Credentials())
	if err != nil {
		return nil, fmt.Errorf("unable to open betting sessions: %w", err)
	}

	d := dispatch.New(h.Engine(), account, *s, a.lookupBetfairRace)
	p := process.New(*s, d, a.lookupBetfairRace, a.setBetfairPriceFeed, func() { a.remove(key) })

	a.mu.Lock()
	a.procs[key] = p
	a.mu.Unlock()

	logger.Debug(logger.Log{
		Application:      core.AppName,
		FormattedMessage: fmt.Sprintf("added process scopes=%v betmatic=%v betfair=%v", activeScopes(s), s.BetmaticCredentials.Username, s.BetfairCredentials.Username),
		UserID:           key.UserID,
		ProcessID:        key.ProcessID,
	})
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

// A scope is only bettable if a feed covers its country and a strategy exists
// for that feed and racing code. Checked at add and restart, where it can still
// be reported to the operator rather than discovered as a quiet afternoon.
func (a *App) validateScopes(s *settings.ProcessSettings) error {
	a.mu.RLock()
	enabled := a.enabled
	a.mu.RUnlock()

	for _, key := range activeScopes(s) {
		country, code, ok := core.SplitScopeKey(key)
		if !ok {
			return fmt.Errorf("unknown scope %q", key)
		}

		provider, ok := core.ProviderFor(country)
		if !ok {
			return fmt.Errorf("no live feed covers %q", country)
		}
		if !enabled[provider] {
			return fmt.Errorf("scope %s: feed %s is disabled", key, provider)
		}

		if !strategy.Covers(provider, code) {
			return fmt.Errorf("scope %s: no strategy for provider %q code %q", key, provider, code)
		}
	}
	return nil
}

// ---- fan-out ----

// onTripleS and onTPD are the feed handlers: every running process whose scope
// matches gets the message on its inbox.
//
// The venue name is canonicalised here, once per packet, so every log line
// downstream names the track the same way.
func (a *App) onTripleS(m triples.RaceMessage) {
	ref, ok := m.Ref()
	if !ok || ref.Scope == "" {
		return
	}
	ref.VenueName = core.CanonicalVenue(ref.Provider, ref.Venue, ref.VenueName)

	wanted := false
	a.mu.RLock()
	for _, p := range a.procs {
		if p.OfferTripleS(m, ref) {
			wanted = true
		}
	}
	a.mu.RUnlock()

	a.afterFanOut(ref, wanted)
}

func (a *App) onTPD(pr tpd.Progress, ref core.RaceRef) {
	if ref.Scope == "" {
		return
	}
	ref.VenueName = core.CanonicalVenue(ref.Provider, ref.Venue, ref.VenueName)

	wanted := false
	a.mu.RLock()
	for _, p := range a.procs {
		if p.OfferTPD(pr, ref) {
			wanted = true
		}
	}
	a.mu.RUnlock()

	a.afterFanOut(ref, wanted)
}

func (a *App) afterFanOut(ref core.RaceRef, wanted bool) {
	if ref.Status == core.StatusFinished {
		a.unmatched.Delete(ref.Key)
		a.betfairMissing.Delete(ref.Key)
		return
	}
	if !wanted {
		a.noMatch(ref)
	}
}

// noMatch is the "data is arriving but nothing bets" case, rate limited to one
// line per race. Every other explanation for a quiet bot is logged elsewhere;
// this covers a race no running process has staked.
func (a *App) noMatch(ref core.RaceRef) {
	if _, seen := a.unmatched.LoadOrStore(ref.Key, struct{}{}); seen {
		return
	}

	logger.Debug(logger.Log{
		Application:      core.AppName,
		FormattedMessage: fmt.Sprintf("race reached no process scope=%v provider=%v status=%v", ref.Scope, ref.Provider, ref.Status),
		RaceDetails:      &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
	})
}

// ---- betfair lookup ----

func (a *App) admin() (*betfair.Client, context.Context) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.betfair, a.ctx
}

// setBetfairPriceFeed turns price polling for a race on or off. Called for
// every message a process handles, so both sides are idempotent and cheap.
func (a *App) setBetfairPriceFeed(ref core.RaceRef, on bool) {
	bf, ctx := a.admin()
	if bf == nil {
		return
	}

	trackName, ok := core.BetfairTrackFor(ref.Provider, ref.Venue)
	if !ok {
		return
	}

	code := core.BetfairRacingCode(ref.Code)

	if on {
		if !bf.HasRace(code, ref.Country, trackName, ref.RaceNumber) {
			a.missingBetfairRace(ref, trackName)
			return
		}
		bf.StartRunnerUpdates(ctx, code, ref.Country, trackName, ref.RaceNumber)
		return
	}

	bf.StopRunnerUpdates(code, ref.Country, trackName, ref.RaceNumber)
}

func (a *App) missingBetfairRace(ref core.RaceRef, trackName string) {
	if _, seen := a.betfairMissing.LoadOrStore(ref.Key, struct{}{}); seen {
		return
	}

	logger.Warn(logger.Log{
		Application:      core.AppName,
		FormattedMessage: fmt.Sprintf("betfair race not loaded; no prices for it scope=%v provider=%v betfair_track=%v", ref.Scope, ref.Provider, trackName),
		RaceDetails:      &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
	})
}

func (a *App) lookupBetfairRace(ref core.RaceRef) *core.BetfairRace {
	bf, _ := a.admin()
	if bf == nil {
		return nil
	}

	trackName, ok := core.BetfairTrackFor(ref.Provider, ref.Venue)
	if !ok {
		return nil
	}

	return bf.GetRace(core.BetfairRacingCode(ref.Code), ref.Country, trackName, ref.RaceNumber)
}

var _ kernel.App = (*App)(nil)
