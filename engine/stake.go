// engine/stake.go

package engine

import "math"

func GetLiability(isMetro, isHorse bool, WMBL, PMBL bool, WLia, PLia float64) (float64, float64) {
	var winLia float64
	var placeLia float64

	if WLia == 0 && !WMBL {
		winLia = 0
	} else {
		if WMBL {
			if isMetro {
				if isHorse {
					winLia = 2000
				} else {
					winLia = 1000
				}
			} else {
				if isHorse {
					winLia = 1000
				} else {
					winLia = 500
				}
			}
		} else {
			winLia = WLia
		}
	}

	if PLia == 0 && !PMBL {
		placeLia = 0
	} else {
		if PMBL {
			if isMetro {
				if isHorse {
					placeLia = 800
				} else {
					placeLia = 400
				}
			} else {
				if isHorse {
					placeLia = 400
				} else {
					placeLia = 200
				}
			}
		} else {
			placeLia = PLia
		}
	}

	return winLia, placeLia
}

var betfairTickLadder = [...]struct{ limit, step int }{
	{600, 10}, {1000, 20}, {2000, 50}, {3000, 100},
	{5000, 200}, {10000, 500}, {100000, 1000},
}

// FloorToBetfairTick rounds down onto the ladder, which is the permissive
// direction for a back limit (the minimum SP you will accept) and the
// restrictive one for a lay.
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

// CeilToBetfairTick rounds up onto the ladder. A lay limit is the maximum SP
// you will accept, so rounding it down would tighten the order rather than
// leave it alone — the opposite of what rounding a back limit down does.
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
