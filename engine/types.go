// engine/types.go

package engine

import (
	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
)

type Order struct {
	Account *Account
	Event   Event
	Side    Side
	Runner  int
	Unit    float64
	Stake   Stake

	// betmatic label or betfair customer ref
	Label string
}

type Event struct {
	Key        string
	VenueName  string
	RaceNumber int
	Country    string
	Code       betmatic.RacingCode

	Betmatic *betmatic.Venue
	Betfair  *betfair.Race
}

type Stake struct {
	Betmatic BetmaticStake `json:"betmatic"`
	Betfair  BetfairStake  `json:"betfair"`
}

type BetmaticStake struct {
	WinStake float64 `json:"win_stake"`
	WinMBL   bool    `json:"win_mbl"`
	MinOdds  float64 `json:"min_odds"`
	MaxOdds  float64 `json:"max_odds"`
}

type BetfairStake struct {
	BackStake float64 `json:"back_stake"`
	LayStake  float64 `json:"lay_stake"`
	MinOdds   float64 `json:"min_odds"`
	MaxOdds   float64 `json:"max_odds"`
}
