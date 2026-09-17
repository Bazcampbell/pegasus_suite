// strategy/types.go

package strategy

import (
	"time"

	triples "racing_wagering/apps/pegasus/triples"
)

// forwardProgressState is per race, held by a Triple-S strategy. initial is the
// message every later displacement is measured against.
type forwardProgressState struct {
	initial        triples.RaceMessage
	hasInitial     bool
	ignore         bool
	betfairPlaced  bool
	betmaticPlaced bool
	distance       int
}

type tpdRaceState struct {
	ignore       bool
	runningSince time.Time

	betfairPlaced  bool
	betmaticPlaced bool
}
