// pegasus/dispatch/dispatch.go
//
// The application's half of placing a bet: name the race the way the
// bookmakers do, attach the live Betfair book, and hand the engine an Order.
// Everything about money and requests lives in the engine.

package dispatch

import (
	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/settings"
	"pegasus_suite/engine"
)

const application = "pegasus"

type Dispatcher struct {
	eng      *engine.Engine
	account  *engine.Account
	settings settings.ProcessSettings

	// nil when there is no Betfair price source (tests)
	getBetfairRace func(core.RaceRef) *core.BetfairRace
}

func New(eng *engine.Engine, account *engine.Account, s settings.ProcessSettings, getBetfairRace func(core.RaceRef) *core.BetfairRace) *Dispatcher {
	return &Dispatcher{eng: eng, account: account, settings: s, getBetfairRace: getBetfairRace}
}

// Place runs on its own goroutine per bet, so the decision path is never
// waiting on a bookmaker.
func (d *Dispatcher) Place(b core.Bet, scope settings.ScopeSettings, strategyCode string) {
	ev := engine.Event{
		Key:        b.Ref.Key,
		VenueName:  b.Ref.VenueName,
		RaceNumber: b.Ref.RaceNumber,
		Country:    b.Ref.Country,
		Code:       b.Ref.Code,
	}

	if v, ok := core.BetmaticVenueFor(b.Ref.Provider, b.Ref.Venue); ok {
		ev.Betmatic = &v
	}

	if b.Side != core.BetmaticWin && d.getBetfairRace != nil {
		ev.Betfair = d.getBetfairRace(b.Ref)
	}

	d.eng.Place(engine.Order{
		Account: d.account,
		Event:   ev,
		Side:    b.Side,
		Runner:  b.Runner,
		Unit:    b.Unit,
		Stake:   scope.Stake,
		Label:   application + "_" + d.settings.ID + "_" + strategyCode,
	})
}
