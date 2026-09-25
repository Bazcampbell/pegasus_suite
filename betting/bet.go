// betting/bet.go

package betting

import "time"

type BetStatus string

const (
	BetPending  BetStatus = "PENDING"
	BetWon      BetStatus = "WON"
	BetLost     BetStatus = "LOST"
	BetVoid     BetStatus = "VOID"
	BetRejected BetStatus = "REJECTED"
	BetLapsed   BetStatus = "LAPSED"
)

// internal bet struct, per book
type BookmakerBet struct {
	ProviderID string // bf ref or betmatic notification ID
	Bookmaker  string // book, bf, tote

	Venue      string // betmatic venue
	RaceNo     string
	RunnerNo   string // de-dupe
	RunnerName string

	PlacedAt   time.Time
	ResultedAt time.Time

	Status    BetStatus
	Requested float64
	Accepted  float64
	Odds      float64
	Return    float64
}

// bet recap for an entire event
type EventBetRecap struct {
	Venue      string
	RaceNo     string
	RunnerNo   string
	RunnerName string

	Bets []BookmakerBet

	Status         BetStatus
	TotalRequested float64
	TotalAccepted  float64
	AvgOdds        float64
	TotalReturn    float64
}

func (b BookmakerBet) Resulted() bool { return b.Status != BetPending }
