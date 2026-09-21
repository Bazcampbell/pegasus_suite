// strategy/types.go

package strategy

import "time"

type tpdRaceState struct {
	ignore       bool
	runningSince time.Time

	betfairPlaced  bool
	betmaticPlaced bool
}
