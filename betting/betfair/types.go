// betfair/types.go

package betfair

import "time"

type RacingCode string

const (
	THOROUGHBRED RacingCode = "Thoroughbred"
	TROT         RacingCode = "Trot"
)

type Event struct {
	Country   string
	TrackName string
	Code      RacingCode
	Races     map[int]*Race // race number → race
}

// Race is one WIN market. Races are replaced, never modified, when the catalogue refreshes.
type Race struct {
	Number    int
	ID        string // market ID
	Name      string
	Distance  int
	StartTime time.Time
	Runners   map[int]Runner // saddlecloth number → runner
}

type Runner struct {
	Number      int
	SelectionID int64
}

type BSPBetRequest struct {
	MarketID    string  `json:"market_id"`
	SelectionID int64   `json:"selection_id"`
	Side        string  `json:"side"`
	Liability   float64 `json:"liability"`
	LimitPrice  float64 `json:"limit_price,omitempty"`

	OrderRef    string `json:"customer_order_ref,omitempty"`    // at most 32 characters
	StrategyRef string `json:"customer_strategy_ref,omitempty"` // at most 15 characters
}
