// pegasus/process/process.go
//
// Process is one bookmaker account pairing. It owns the inbox, the strategies
// serving its scopes, and the dispatcher holding its sessions. It knows nothing
// about how a bet is placed.

package process

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/dispatch"
	"pegasus_suite/apps/pegasus/settings"
	"pegasus_suite/apps/pegasus/strategy"
	triples "pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/logger"
)

const inboxSize = 100

type tripleSMsg struct {
	m   triples.RaceMessage
	ref core.RaceRef
}

type Process struct {
	Settings settings.ProcessSettings

	// Fed by the application's fan-out. Buffered so a slow decision never
	// blocks the feed; a full inbox drops.
	tripleS chan tripleSMsg

	dispatcher     *dispatch.Dispatcher
	getBetfairRace func(core.RaceRef) *core.BetfairRace
	onClose        func()

	running atomic.Bool

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	// Only the run goroutine touches these.
	forwardProgress *strategy.ForwardProgress

	// Races this process has already reported on, so the first update for a race
	// logs what it decided and the ticks that follow stay quiet.
	seenRaces map[string]bool
}

func New(s settings.ProcessSettings, d *dispatch.Dispatcher, getBetfairRace func(core.RaceRef) *core.BetfairRace, onClose func()) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // start stopped

	return &Process{
		Settings:        s,
		tripleS:         make(chan tripleSMsg, inboxSize),
		dispatcher:      d,
		getBetfairRace:  getBetfairRace,
		onClose:         onClose,
		forwardProgress: strategy.NewForwardProgress(),
		seenRaces:       make(map[string]bool),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Wants runs on the fan-out goroutine, so it only reads Settings, which is
// immutable for the life of the process. One map lookup, no decode.
func (p *Process) Wants(ref core.RaceRef) bool {
	if !p.running.Load() {
		return false
	}
	scope, ok := p.Settings.Scopes[ref.Scope]
	return ok && scope.Active()
}

func (p *Process) OfferTripleS(m triples.RaceMessage, ref core.RaceRef) bool {
	if !p.Wants(ref) {
		return false
	}
	select {
	case p.tripleS <- tripleSMsg{m: m, ref: ref}:
	default:
		p.inboxFull(ref)
	}
	return true
}

func (p *Process) inboxFull(ref core.RaceRef) {
	logger.Warn(logger.Log{
		App:       core.AppName,
		Message:   "race inbox full",
		UserID:    p.Settings.UserID,
		ProcessID: p.Settings.ID,
		Race:      ref.LogRace(),
	})
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
	p.running.Store(true)

	logger.Debug(logger.Log{App: core.AppName, Message: "starting process", ProcessID: p.Settings.ID, UserID: p.Settings.UserID})

	go p.run(ctx)
}

func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctx.Err() != nil {
		return
	}
	p.running.Store(false)
	p.cancel()
}

func (p *Process) Running() bool { return p.running.Load() }

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
			logger.Info(logger.Log{
				App:       core.AppName,
				Message:   fmt.Sprintf("process stopped reason=%v", ctx.Err()),
				UserID:    p.Settings.UserID,
				ProcessID: p.Settings.ID,
			})
			return

		case msg := <-p.tripleS:
			p.handle(msg)
		}
	}
}

// handle runs one message through the strategy and places what it selects.
func (p *Process) handle(msg tripleSMsg) {
	defer logger.Recover(core.AppName)
	scope, ok := p.scopeFor(msg.ref)
	if !ok {
		return
	}
	bets, err := p.forwardProgress.Select(msg.m, msg.ref, scope.BetfairDelay, scope.BetmaticDelay, p.getBetfairRace)
	p.act(msg.ref, scope, bets, err)
}

func (p *Process) scopeFor(ref core.RaceRef) (settings.ScopeSettings, bool) {
	scope, ok := p.Settings.Scopes[ref.Scope]
	if !ok || !scope.Active() {
		return scope, false
	}

	if ref.Status == core.StatusFinished {
		delete(p.seenRaces, ref.Key)
	} else if !p.seenRaces[ref.Key] {
		// First update for this race only: what it matched and what it will
		// stake. Anything logged unconditionally here buries everything else.
		p.seenRaces[ref.Key] = true
		logger.Debug(logger.Log{
			App:       core.AppName,
			Message:   fmt.Sprintf("race in scope=%v status=%v bm_stake=%.2f bm_mbl=%v bm_delay=%v bf_back=%.2f bf_lay=%.2f bf_delay=%v", ref.Scope, ref.Status, scope.Betmatic.WinStake, scope.Betmatic.WinMBL, scope.BetmaticDelay, scope.Betfair.BackStake, scope.Betfair.LayStake, scope.BetfairDelay),
			UserID:    p.Settings.UserID,
			ProcessID: p.Settings.ID,
			Race:      ref.LogRace(),
		})
	}
	return scope, true
}

// act hands a strategy's bets on. Every bet goes out on its own goroutine
// so no bookmaker can hold up the next message.
func (p *Process) act(ref core.RaceRef, scope settings.ScopeSettings, bets []core.Bet, err error) {
	if err != nil {
		logger.Error(logger.Log{
			App:       core.AppName,
			Message:   fmt.Sprintf("unable to make selections error=%v", err),
			UserID:    p.Settings.UserID,
			ProcessID: p.Settings.ID,
			Race:      ref.LogRace(),
		})
		return
	}

	if len(bets) == 0 {
		return
	}

	logger.Debug(logger.Log{
		App:       core.AppName,
		Message:   fmt.Sprintf("selections made bets=%+v", bets),
		UserID:    p.Settings.UserID,
		ProcessID: p.Settings.ID,
		Race:      ref.LogRace(),
	})

	for _, b := range bets {
		go p.dispatcher.Place(b, scope)
	}
}
