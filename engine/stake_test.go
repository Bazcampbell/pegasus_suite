package engine

import "testing"

// Every value the tick helpers return is handed straight to betfair as an
// order price, so landing between ticks is a rejected bet. The ladder is
// deliberately coarser than betfair's own below $6 — it only ever emits
// multiples of $0.10 there, all of which are valid ticks.
func TestBetfairTicks(t *testing.T) {
	cases := []struct {
		price       float64
		floor, ceil float64
	}{
		{1.00, 0, 0},
		{1.005, 0, 1.10},
		{1.01, 0, 1.10},
		{1.10, 1.10, 1.10},
		{2.53, 2.50, 2.60},
		{5.95, 5.90, 6.00},
		{6.00, 6.00, 6.00},
		{6.01, 6.00, 6.20},
		{9.99, 9.80, 10.00},
		{25.00, 25.00, 25.00},
		{25.10, 25.00, 26.00},
		{999.00, 990.00, 1000.00},
		{1200.00, 1000.00, 1000.00},
	}

	for _, c := range cases {
		if got := FloorToBetfairTick(c.price); got != c.floor {
			t.Errorf("FloorToBetfairTick(%.2f) = %.2f, want %.2f", c.price, got, c.floor)
		}
		if got := CeilToBetfairTick(c.price); got != c.ceil {
			t.Errorf("CeilToBetfairTick(%.2f) = %.2f, want %.2f", c.price, got, c.ceil)
		}
	}
}

// A lay limit must never come back tighter than the price asked for, or the
// rounding itself would cost matches.
func TestCeilNeverTightensLay(t *testing.T) {
	for cents := 101; cents <= 100000; cents++ {
		price := float64(cents) / 100
		if got := CeilToBetfairTick(price); got < price {
			t.Fatalf("CeilToBetfairTick(%.2f) = %.2f, below the price asked for", price, got)
		}
	}
}
