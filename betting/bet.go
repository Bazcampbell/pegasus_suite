// betting/bet.go

package betting

type BetStatus string

const (
	BetPending BetStatus = "PENDING"
	BetWon     BetStatus = "WON"
	BetLost    BetStatus = "LOST"
	BetVoid    BetStatus = "VOID"
	BetLapsed  BetStatus = "LAPSED" // never matched, cancelled or rejected
)

// Bet is one provider bet as the provider reports it.
type Bet struct {
	ID        string
	Provider  Provider
	Status    BetStatus
	Liability float64 // what was at risk; 0 unless matched and not void
	Profit    float64 // settled profit; 0 until settled
	Bot       string  // Betmatic bot ID; empty for Betfair
}
