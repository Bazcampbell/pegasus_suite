// betfair/bet.go

package betfair

import (
	"fmt"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
)

// clearedStatuses is the order GetBet asks in: a settled bet is the common
// case, the rest are final with nothing won or lost.
var clearedStatuses = []exchange.BetStatus{
	exchange.BetStatusSettled,
	exchange.BetStatusVoided,
	exchange.BetStatusLapsed,
	exchange.BetStatusCancelled,
}

// GetBet results one bet by betId (listClearedOrders). A bet not yet cleared
// under any status is still pending.
func (bc *Client) GetBet(id string) (betting.Bet, error) {
	for _, status := range clearedStatuses {
		report, err := bc.api.ListClearedOrders(exchange.ListClearedOrdersRequest{
			BetStatus: status,
			BetIDs:    []string{id},
		})
		if err != nil {
			return betting.Bet{}, fmt.Errorf("betfair listClearedOrders %s %s: %w", status, id, err)
		}
		for _, o := range report.ClearedOrders {
			if o.BetID == id {
				return clearedBet(status, o), nil
			}
		}
	}
	return betting.Bet{ID: id, Provider: betting.ProviderBetfair, Status: betting.BetPending}, nil
}

func clearedBet(status exchange.BetStatus, o exchange.ClearedOrderSummary) betting.Bet {
	b := betting.Bet{
		ID:        o.BetID,
		Provider:  betting.ProviderBetfair,
		Stake:     o.SizeSettled,
		Odds:      o.PriceMatched,
		Profit:    o.Profit,
		SettledAt: o.SettledDate,
	}
	switch {
	case status == exchange.BetStatusLapsed || status == exchange.BetStatusCancelled:
		b.Status, b.Stake, b.Profit = betting.BetLapsed, 0, 0
	case status == exchange.BetStatusVoided:
		b.Status, b.Profit = betting.BetVoid, 0
	case o.Profit > 0:
		b.Status = betting.BetWon
	case o.Profit < 0:
		b.Status = betting.BetLost
	default:
		b.Status = betting.BetVoid
	}
	return b
}
