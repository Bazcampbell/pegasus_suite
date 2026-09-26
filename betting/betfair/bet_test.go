package betfair

import (
	"testing"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
)

func TestClearedBetLiability(t *testing.T) {
	back := clearedBet(exchange.BetStatusSettled, exchange.ClearedOrderSummary{Side: exchange.SideBack, SizeSettled: 10, PriceMatched: 4, Profit: 30})
	lay := clearedBet(exchange.BetStatusSettled, exchange.ClearedOrderSummary{Side: exchange.SideLay, SizeSettled: 10, PriceMatched: 4, Profit: -30})
	lapsed := clearedBet(exchange.BetStatusLapsed, exchange.ClearedOrderSummary{SizeSettled: 10})

	if back.Status != betting.BetWon || back.Liability != 10 || back.Profit != 30 {
		t.Errorf("back = %+v", back)
	}
	if lay.Status != betting.BetLost || lay.Liability != 30 || lay.Profit != -30 {
		t.Errorf("lay = %+v, want liability stake x (price - 1)", lay)
	}
	if lapsed.Status != betting.BetLapsed || lapsed.Liability != 0 {
		t.Errorf("lapsed = %+v", lapsed)
	}
}
