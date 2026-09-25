// pegasus/dispatch/dispatch.go

package dispatch

import (
	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/settings"
	"pegasus_suite/engine"
	"pegasus_suite/logger"
)

type Dispatcher struct {
	eng         *engine.Engine
	account     *engine.Account
	betfairRace func(core.RaceRef) *core.BetfairRace
}

func New(eng *engine.Engine, account *engine.Account, betfairRace func(core.RaceRef) *core.BetfairRace) *Dispatcher {
	return &Dispatcher{eng: eng, account: account, betfairRace: betfairRace}
}

// Place resolves b's Betmatic venue and Betfair IDs and hands the engine the order.
// It blocks for the bookmaker, so callers run it on its own goroutine.
func (d *Dispatcher) Place(b core.Bet, scope settings.ScopeSettings) {
	venue, mapped := core.BetmaticVenueFor(b.Ref.Venue)
	if b.Side == core.BetmaticWin && !mapped {
		logger.Error(logger.Log{App: core.AppName, UserID: d.account.UserID, ProcessID: d.account.ProcessID, Race: b.Ref.LogRace(), Message: "no betmatic venue for track; cannot bet"})
		return
	}

	o := engine.Order{
		Account: d.account,
		Race: engine.Race{
			Date:   b.Ref.Date,
			Venue:  b.Ref.VenueName,
			Metro:  venue.IsMetro,
			Code:   b.Ref.Code,
			Number: b.Ref.RaceNumber,
		},
		Runner: b.Runner,
		Side:   b.Side,
		Unit:   b.Unit,
		Stake:  scope.Stake,
	}
	if race := d.betfairRace(b.Ref); race != nil {
		o.Race.MarketID = race.ID
		o.SelectionID = race.Runners[b.Runner].SelectionID
	}
	d.eng.Place(o)
}
