// betfair/bet.go

package betfair

import (
	"fmt"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
)

var clearedStatuses = []exchange.BetStatus{
	exchange.BetStatusSettled,
	exchange.BetStatusVoided,
	exchange.BetStatusLapsed,
	exchange.BetStatusCancelled,
}

// SettledBets returns every bet on the account cleared in [from, to), lapsed and cancelled included.
func (bc *Client) SettledBets(from, to time.Time) ([]betting.Bet, error) {
	var out []betting.Bet
	for _, status := range clearedStatuses {
		for fetched := 0; ; {
			report, err := bc.api.ListClearedOrders(exchange.ListClearedOrdersRequest{
				BetStatus:        status,
				SettledDateRange: &exchange.TimeRange{From: from, To: to},
				FromRecord:       fetched,
			})
			if err != nil {
				return nil, fmt.Errorf("betfair listClearedOrders %s: %w", status, err)
			}
			for _, o := range report.ClearedOrders {
				out = append(out, clearedBet(status, o))
			}
			fetched += len(report.ClearedOrders)
			if !report.MoreAvailable || len(report.ClearedOrders) == 0 {
				break
			}
		}
	}
	return out, nil
}

func clearedBet(status exchange.BetStatus, o exchange.ClearedOrderSummary) betting.Bet {
	b := betting.Bet{ID: o.BetID, Provider: betting.ProviderBetfair}
	switch {
	case status == exchange.BetStatusLapsed || status == exchange.BetStatusCancelled:
		b.Status = betting.BetLapsed
		return b
	case status == exchange.BetStatusVoided:
		b.Status = betting.BetVoid
		return b
	case o.Profit > 0:
		b.Status = betting.BetWon
	case o.Profit < 0:
		b.Status = betting.BetLost
	default:
		b.Status = betting.BetVoid
		return b
	}
	b.Profit = o.Profit
	b.Liability = o.SizeSettled
	if o.Side == exchange.SideLay {
		b.Liability = o.SizeSettled * (o.PriceMatched - 1)
	}
	return b
}
