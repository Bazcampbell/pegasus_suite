// pegasus/process/process.go
//
// Process is one bookmaker account pairing. It owns the inbox, the strategies
// serving its scopes, and the dispatcher holding its sessions. It knows nothing
// about how a bet is placed and nothing about any feed's wire format.

package process

import (
	"context"
	"fmt"
	"sync"

	"racing_wagering/apps/pegasus/core"
	"racing_wagering/apps/pegasus/dispatch"
	"racing_wagering/apps/pegasus/settings"
	"racing_wagering/apps/pegasus/strategy"
	"racing_wagering/logger"
)

type Process struct {
	Settings settings.ProcessSettings

	// Inbox is fed by the application's fan-out. Buffered so a slow decision
	// never blocks the feed; a full inbox drops.
	Inbox chan core.Update

	dispatcher     *dispatch.Dispatcher
	getBetfairRace func(core.RaceRef) *core.BetfairRace
	setPriceFeed   func(core.RaceRef, bool)
	onClose        func()

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	// One strategy per scope, built on first use. Each holds its own per-race
	// state, so two scopes can never contaminate one another, and two processes
	// on the same race never share a "already bet" flag. Only the run goroutine
	// touches this.
	strategies map[string]strategy.Strategy

	// Races this process has already reported on, so the first update for a race
	// logs what it decided and the ticks that follow stay quiet.
	seenRaces map[string]bool
}

func New(s settings.ProcessSettings, d *dispatch.Dispatcher, getBetfairRace func(core.RaceRef) *core.BetfairRace, setPriceFeed func(core.RaceRef, bool), onClose func()) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // start stopped

	return &Process{
		Settings:       s,
		Inbox:          make(chan core.Update, 100),
		dispatcher:     d,
		getBetfairRace: getBetfairRace,
		setPriceFeed:   setPriceFeed,
		onClose:        onClose,
		strategies:     make(map[string]strategy.Strategy),
		seenRaces:      make(map[string]bool),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// WantsMessage runs on the fan-out goroutine, so it only reads Settings, which
// is immutable for the life of the process. One map lookup, no decode.
func (p *Process) WantsMessage(u core.Update) bool {
	scope, ok := p.Settings.Scopes[u.Ref.Scope]
	return ok && scope.Active()
}

func (p *Process) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = ctx
	p.cancel = cancel

	logger.Debug(logger.InfoLog{Message: "starting process", ProcessID: p.Settings.ID, UserID: p.Settings.UserID})

	go p.run(ctx)
}

func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() != nil {
		return
	}
	p.cancel()
}

func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ctx.Err() == nil
}

// Close removes the process from the application's fan-out. The kernel
// releases its sessions afterwards.
func (p *Process) Close() {
	p.Stop()
	if p.onClose != nil {
		p.onClose()
	}
}

// run is the per-Start goroutine. It owns the ctx passed to it (a snapshot of
// p.ctx at Start time), so later Stop/Start cycles can reassign p.ctx without
// affecting the still-draining goroutine.
func (p *Process) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.InfoLog{
				Message:   fmt.Sprintf("process stopped reason=%v", ctx.Err()),
				UserID:    p.Settings.UserID,
				ProcessID: p.Settings.ID,
			})
			return

		case u := <-p.Inbox:
			p.handleUpdate(u)
		}
	}
}

// strategyFor resolves the strategy serving a scope. The pairing of feed and
// racing code decides it; a process no longer chooses.
func (p *Process) strategyFor(ref core.RaceRef) (strategy.Strategy, error) {
	if strat, ok := p.strategies[ref.Scope]; ok {
		return strat, nil
	}

	strat, err := strategy.For(ref.Provider, ref.Code)
	if err != nil {
		return nil, err
	}

	logger.Debug(logger.InfoLog{
		Message:   "resolved strategy=" + strat.Name() + " for scope=" + ref.Scope,
		UserID:    p.Settings.UserID,
		ProcessID: p.Settings.ID,
	})

	p.strategies[ref.Scope] = strat
	return strat, nil
}

// handleUpdate is the whole of a process's decision path: find the scope, ask
// the strategy, hand whatever comes back to dispatch. Every bet goes out on its
// own goroutine so no bookmaker can hold up the next message.
func (p *Process) handleUpdate(u core.Update) {
	scope, ok := p.Settings.Scopes[u.Ref.Scope]
	if !ok || !scope.Active() {
		return
	}

	race := &logger.RaceDetails{Venue: u.Ref.VenueName, RaceNumber: u.Ref.RaceNumber}

	if u.Ref.Status == core.StatusFinished {
		delete(p.seenRaces, u.Ref.Key)
	} else if !p.seenRaces[u.Ref.Key] {
		// First update for this race only: what it matched and what it will
		// stake. Anything logged unconditionally here buries everything else.
		p.seenRaces[u.Ref.Key] = true
		logger.Debug(logger.InfoLog{
			Message:     fmt.Sprintf("race in scope=%v provider=%v status=%v bm_stake=%.2f bm_mbl=%v bm_delay=%v bf_back=%.2f bf_lay=%.2f bf_delay=%v", u.Ref.Scope, u.Ref.Provider, u.Ref.Status, scope.Betmatic.WinStake, scope.Betmatic.WinMBL, scope.BetmaticDelay, scope.Betfair.BackStake, scope.Betfair.LayStake, scope.BetfairDelay),
			UserID:      p.Settings.UserID,
			ProcessID:   p.Settings.ID,
			RaceDetails: race,
		})
	}

	strat, err := p.strategyFor(u.Ref)
	if err != nil {
		logger.Error(logger.ErrorLog{
			Message:     fmt.Sprintf("unable to resolve strategy error=%v", err),
			UserID:      p.Settings.UserID,
			ProcessID:   p.Settings.ID,
			RaceDetails: race,
		})
		return
	}

	decision, err := strat.Select(u, scope.BetfairDelay, scope.BetmaticDelay, p.getBetfairRace)
	if err != nil {
		logger.Error(logger.ErrorLog{
			Message:     fmt.Sprintf("unable to make selections error=%v", err),
			UserID:      p.Settings.UserID,
			ProcessID:   p.Settings.ID,
			RaceDetails: race,
		})
		return
	}

	// nil when the process has no betfair account, so there are no prices to poll
	if p.setPriceFeed != nil {
		p.setPriceFeed(u.Ref, decision.Tracking)
	}

	if len(decision.Bets) == 0 {
		return
	}

	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("selections made strategy=%v bets=%+v", strat.Code(), decision.Bets),
		UserID:      p.Settings.UserID,
		ProcessID:   p.Settings.ID,
		RaceDetails: race,
	})

	for _, b := range decision.Bets {
		go p.dispatcher.Place(b, scope, strat.Code())
	}
}
