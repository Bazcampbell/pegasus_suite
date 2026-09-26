// engine/types.go

package engine

import (
	"fmt"
	"strings"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betmatic"
)

type Side int

const (
	BetmaticWin Side = iota
	BetmaticPlace
	BetfairBack
	BetfairLay
)

func (s Side) String() string {
	switch s {
	case BetmaticWin:
		return "betmatic-win"
	case BetmaticPlace:
		return "betmatic-place"
	case BetfairBack:
		return "betfair-back"
	case BetfairLay:
		return "betfair-lay"
	default:
		return "unknown"
	}
}

func (s Side) provider() betting.Provider {
	if s == BetmaticWin || s == BetmaticPlace {
		return betting.ProviderBetmatic
	}
	return betting.ProviderBetfair
}

// Order is one bet on one runner with one provider. The app resolves every name and ID.
type Order struct {
	Account     *Account
	Race        Race
	Runner      int
	SelectionID int64 // Betfair selection; 0 when the app has none
	Side        Side
	Unit        float64
	Stake       Stake
}

type Race struct {
	Date     string // race date in AEST, YYYY-MM-DD
	Venue    string // Betmatic name
	Metro    bool
	Code     betmatic.RacingCode
	Number   int
	MarketID string // Betfair WIN market; "" when the app has none
}

// BetID returns VENUE:R<race>:<runner>:<YYYYMMDD>, the runner's identity across every provider.
func (o Order) BetID() string {
	return fmt.Sprintf("%s:R%d:%d:%s", strings.ReplaceAll(o.Race.Venue, " ", ""), o.Race.Number, o.Runner, strings.ReplaceAll(o.Race.Date, "-", ""))
}

func (o Order) staked() bool {
	switch o.Side {
	case BetmaticWin, BetmaticPlace:
		return o.Stake.BetsBetmatic()
	case BetfairBack:
		return o.Stake.Betfair.BackStake > 0
	case BetfairLay:
		return o.Stake.Betfair.LayStake > 0
	}
	return false
}

type Stake struct {
	Betmatic BetmaticStake `json:"betmatic"`
	Betfair  BetfairStake  `json:"betfair"`
}

// BetmaticStake targets WinStake × unit profit, or with Cash stakes WinStake × unit dollars at the highest odds.
type BetmaticStake struct {
	WinStake float64 `json:"win_stake"`
	WinMBL   bool    `json:"win_mbl"`
	MinOdds  float64 `json:"min_odds"`
	MaxOdds  float64 `json:"max_odds"`
	Cash     bool    `json:"cash,omitempty"`
}

type BetfairStake struct {
	BackStake float64 `json:"back_stake"`
	LayStake  float64 `json:"lay_stake"`
	MinOdds   float64 `json:"min_odds"`
	MaxOdds   float64 `json:"max_odds"`
}

// Active reports whether anything is staked. MBL counts with a zero stake because it bets the bookmaker maximum.
func (s Stake) Active() bool { return s.BetsBetmatic() || s.BetsBetfair() }

func (s Stake) BetsBetmatic() bool { return s.Betmatic.WinMBL || s.Betmatic.WinStake > 0 }

func (s Stake) BetsBetfair() bool { return s.Betfair.BackStake > 0 || s.Betfair.LayStake > 0 }
