// engine/stake.go

package engine

import "math"

// maxBetLiability returns the MBL target for a track: metro over provincial, thoroughbred over harness.
func maxBetLiability(metro, thoroughbred bool) float64 {
	switch {
	case metro && thoroughbred:
		return 2000
	case metro:
		return 1000
	case thoroughbred:
		return 1000
	default:
		return 500
	}
}

var betfairTickLadder = [...]struct{ limit, step int }{
	{600, 10}, {1000, 20}, {2000, 50}, {3000, 100},
	{5000, 200}, {10000, 500}, {100000, 1000},
}

// rounds down into the ladder to nearest valid tick
func FloorToBetfairTick(price float64) float64 {
	cents := int(math.Floor(price*100 + 1e-9)) // binary float lift
	if cents > 100000 {
		cents = 100000
	}

	anchor := 100
	for _, b := range betfairTickLadder {
		if cents < b.limit {
			floored := anchor + (cents-anchor)/b.step*b.step
			// under $1.01 odds
			if floored < 101 {
				return 0
			}
			return float64(floored) / 100
		}
		anchor = b.limit
	}

	return float64(cents) / 100
}

// rounds up to the nearest valid tick
func CeilToBetfairTick(price float64) float64 {
	cents := int(math.Ceil(price*100 - 1e-9))
	if cents < 101 {
		return 0
	}
	if cents >= 100000 {
		return 1000
	}

	anchor := 100
	for _, b := range betfairTickLadder {
		if cents <= b.limit {
			ceiled := anchor + (cents-anchor+b.step-1)/b.step*b.step
			return float64(ceiled) / 100
		}
		anchor = b.limit
	}

	return float64(cents) / 100
}
