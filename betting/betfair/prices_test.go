package betfair

import (
	"testing"

	"pegasus_suite/betting/betfair/internal/exchange"
)

func TestApplyMarketChange(t *testing.T) {
	bc := &Client{}
	var closed []string
	onClosed := func(id string) { closed = append(closed, id) }

	bc.applyMarketChange(exchange.MarketChange{ID: "1.1", Img: true, RC: []exchange.RunnerChange{
		{ID: 7, LTP: 3.5, BATB: [][3]float64{{0, 3.4, 100}, {1, 3.3, 50}}, BATL: [][3]float64{{0, 3.6, 80}}},
	}}, onClosed)
	bc.applyMarketChange(exchange.MarketChange{ID: "1.1", RC: []exchange.RunnerChange{
		{ID: 7, BATB: [][3]float64{{1, 0, 0}}, BATL: [][3]float64{{0, 3.55, 20}}},
	}}, onClosed)

	r, ok := bc.Runner("1.1", 7)
	if !ok {
		t.Fatal("runner missing")
	}
	if r.LTP != 3.5 || r.Back[0] != (Level{3.4, 100}) || r.Back[1] != (Level{}) || r.Lay[0] != (Level{3.55, 20}) {
		t.Fatalf("prices = %+v", r)
	}

	bc.applyMarketChange(exchange.MarketChange{ID: "1.1", MarketDefinition: &exchange.StreamMarketDefinition{Status: exchange.MarketStatusClosed}}, onClosed)
	if _, ok := bc.Runner("1.1", 7); ok || len(closed) != 1 || closed[0] != "1.1" {
		t.Fatalf("closed market kept: closed=%v", closed)
	}
}
