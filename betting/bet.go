// betting/bet.go

package betting

import "time"

// BetStatus is where a bet a provider accepted has got to.
type BetStatus string

const (
	BetPending BetStatus = "PENDING" // accepted, not resulted yet
	BetWon     BetStatus = "WON"
	BetLost    BetStatus = "LOST"
	BetVoid    BetStatus = "VOID"   // resulted with nothing won or lost: voided, scratched, refunded
	BetLapsed  BetStatus = "LAPSED" // nothing was accepted or matched, so no money was on
)

// Bet is what a provider says about one bet, by the ID it gave on placement:
// the Betmatic notification id or the Betfair betId. Same shape for every
// provider, so the resulter treats them alike.
type Bet struct {
	ID       string
	Provider Provider
	Status   BetStatus

	Stake  float64 // money actually on: Betmatic accepted, Betfair size settled
	Odds   float64 // average price matched
	Profit float64 // positive won, negative lost; Betfair's is before commission

	SettledAt time.Time // zero while pending, or when the provider doesn't say
}

func (b Bet) Resulted() bool { return b.Status != BetPending }
