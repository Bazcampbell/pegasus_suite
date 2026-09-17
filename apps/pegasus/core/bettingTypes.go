// packages/core/bettingTypes.go

package core

import (
	"racing_wagering/betting/betfair"
	"racing_wagering/engine"
)

// The engine owns what a bet is; these aliases keep the strategies and the
// venue tables speaking the same vocabulary without importing it everywhere.
type (
	BetmaticVenue = engine.BetmaticVenue
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

// strategy response
type Decision struct {
	Bets []Bet

	// used for BF price poll
	Tracking bool
}
