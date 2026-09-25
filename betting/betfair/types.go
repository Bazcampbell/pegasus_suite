// betfair/types.go

package betfair

import (
	"time"

	"pegasus_suite/betting/betfair/internal/exchange"
)

type RacingCode string

const (
	THOROUGHBRED RacingCode = "Thoroughbred"
	TROT         RacingCode = "Trot"
)

type Event struct {
	Country   string
	TrackName string
	Code      RacingCode
	Races     map[int]*Race // RaceNumber:Race
}

type Race struct {
	Number   int
	ID       string
	Name     string
	Distance int
	Runners  map[int]*Runner // RunnerNumber:Runner
}

func cloneRace(race *Race) *Race {
	out := *race
	out.Runners = make(map[int]*Runner, len(race.Runners))
	for number, runner := range race.Runners {
		r := *runner
		r.Back = append([]OrderBook(nil), runner.Back...)
		r.Lay = append([]OrderBook(nil), runner.Lay...)
		out.Runners[number] = &r
	}
	return &out
}

type Runner struct {
	Number int
	Lay    []OrderBook // ordered best-worst by default
	Back   []OrderBook
	LTP    float64

	SelectionID string
	MarketID    string
}

type OrderBook struct {
	Price float64
	Size  float64
}

type BetRequest struct {
	MarketID    string  `json:"market_id"`
	SelectionID int64   `json:"selection_id"`
	Side        string  `json:"side"`
	Price       float64 `json:"price"`
	Size        float64 `json:"size"`
	Handicap    float64 `json:"handicap,omitempty"`

	PersistenceType     string `json:"persistence_type,omitempty"`
	CustomerRef         string `json:"customer_ref,omitempty"` // used for de-dupe
	OrderRef            string `json:"customer_order_ref,omitempty"`
	CustomerStrategyRef string `json:"customer_strategy_ref,omitempty"`
}

type BSPBetRequest struct {
	MarketID    string  `json:"market_id"`
	SelectionID int64   `json:"selection_id"`
	Side        string  `json:"side"`
	Liability   float64 `json:"liability"`
	LimitPrice  float64 `json:"limit_price,omitempty"`
	Handicap    float64 `json:"handicap,omitempty"`

	CustomerRef         string `json:"customer_ref,omitempty"` // usesd for de-dupe
	OrderRef            string `json:"customer_order_ref,omitempty"`
	CustomerStrategyRef string `json:"customer_strategy_ref,omitempty"`
}

type BetResult struct {
	BetID      string
	Status     string
	PlacedDate time.Time
}

const (
	SideBack = string(exchange.SideBack)
	SideLay  = string(exchange.SideLay)
)

// returns empty if not at least 3 levels of open orders
func (b Runner) BackWAP() float64 { return wap(b.Back) }

func (b Runner) LayWAP() float64 { return wap(b.Lay) }

func wap(book []OrderBook) float64 {
	const levels = 3

	if len(book) < levels {
		return 0
	}

	var totalPriceSize, totalSize float64
	for i := 0; i < levels && i < len(book); i++ {
		totalPriceSize += book[i].Price * book[i].Size
		totalSize += book[i].Size
	}

	if totalSize == 0 {
		return 0
	}
	return totalPriceSize / totalSize
}
