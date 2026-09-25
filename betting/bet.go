// betting/bet.go

package betting

import "time"

type BetStatus string

const (
	BetPending BetStatus = "PENDING"
	BetWon     BetStatus = "WON"
	BetLost    BetStatus = "LOST"
	BetVoid    BetStatus = "VOID"
	BetLapsed  BetStatus = "LAPSED"
)

// Bet is one provider bet as the provider settled it.
type Bet struct {
	ID        string
	Provider  Provider
	Status    BetStatus
	Stake     float64
	Odds      float64
	Profit    float64
	SettledAt time.Time
}
