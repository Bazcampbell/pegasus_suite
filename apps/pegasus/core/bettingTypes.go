// packages/core/bettingTypes.go

package core

import (
	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/engine"
)

// The engine owns what a bet is; these aliases keep the strategies and the
// venue tables speaking the same vocabulary without importing it everywhere.
type (
	BetmaticVenue = betmatic.Venue
	BetfairRace   = betfair.Race
	Side          = engine.Side
)

const (
	BetmaticWin = engine.BetmaticWin
	BetfairBack = engine.BetfairBack
	BetfairLay  = engine.BetfairLay
)

// strategy produces, dispatch consumes
type Bet struct {
	Ref    RaceRef
	Side   Side
	Runner int
	Unit   float64
}
