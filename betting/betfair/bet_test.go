package betfair

import (
	"encoding/json"
	"testing"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
)

// The shape of a listClearedOrders response for one settled lay bet.
const clearedReport = `{
  "clearedOrders": [{
    "eventTypeId": "7", "eventId": "29821453", "marketId": "1.109850906",
    "selectionId": 237486, "handicap": 0, "betId": "31242604945",
    "placedDate": "2013-10-30T14:22:47.000Z", "persistenceType": "LAPSE",
    "orderType": "LIMIT", "side": "LAY", "betOutcome": "WON",
    "priceRequested": 3.0, "settledDate": "2013-10-30T14:30:02.000Z",
    "lastMatchedDate": "2013-10-30T14:22:47.000Z", "betCount": 1,
    "priceMatched": 3.0, "priceReduced": false, "sizeSettled": 2.0, "profit": 2.0
  }],
  "moreAvailable": false
}`

func TestClearedReportDecodesToABet(t *testing.T) {
	var report exchange.ClearedOrderSummaryReport
	if err := json.Unmarshal([]byte(clearedReport), &report); err != nil {
		t.Fatal(err)
	}
	b := clearedBet(exchange.BetStatusSettled, report.ClearedOrders[0])

	settled := time.Date(2013, 10, 30, 14, 30, 2, 0, time.UTC)
	if b.ID != "31242604945" || b.Provider != betting.ProviderBetfair || b.Status != betting.BetWon ||
		b.Stake != 2 || b.Odds != 3 || b.Profit != 2 || !b.SettledAt.Equal(settled) {
		t.Fatalf("got %+v", b)
	}
}

func TestClearedBetStatuses(t *testing.T) {
	o := exchange.ClearedOrderSummary{BetID: "1", SizeSettled: 10, PriceMatched: 2.5}
	cases := []struct {
		status exchange.BetStatus
		profit float64
		want   betting.BetStatus
		stake  float64
	}{
		{exchange.BetStatusSettled, 15, betting.BetWon, 10},
		{exchange.BetStatusSettled, -10, betting.BetLost, 10},
		{exchange.BetStatusSettled, 0, betting.BetVoid, 10},
		{exchange.BetStatusVoided, 0, betting.BetVoid, 10},
		{exchange.BetStatusLapsed, 0, betting.BetLapsed, 0},
		{exchange.BetStatusCancelled, 0, betting.BetLapsed, 0},
	}
	for _, c := range cases {
		o.Profit = c.profit
		b := clearedBet(c.status, o)
		if b.Status != c.want || b.Stake != c.stake {
			t.Errorf("%s profit %v: got %s stake %v, want %s stake %v", c.status, c.profit, b.Status, b.Stake, c.want, c.stake)
		}
	}
}
