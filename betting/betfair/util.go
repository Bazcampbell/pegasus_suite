// betfair/util.go

package betfair

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"pegasus_suite/betting/betfair/internal/exchange"
)

// betfair has no harness event type: a trot or pace meeting comes back under
// event type 7 like the gallops, and the event itself is named for the track
// alone ("Newcastle (AUS)") — the only marker is in the market names, which
// always carry "Pace" or "Trot" for harness and never do for thoroughbreds.
//
// Every market in a meeting is the same code, so the first one carrying the
// marker settles it. Scanning them all rather than only the lowest-numbered
// race keeps this right when an early market has already closed and is no
// longer returned.
func racingCode(races map[int]*Race) RacingCode {
	for _, race := range races {
		if harnessMarketRe.MatchString(race.Name) {
			return TROT
		}
	}

	return THOROUGHBRED
}

func parseTrackName(eventName string) string {
	if i := strings.Index(eventName, " ("); i != -1 {
		return strings.TrimSpace(eventName[:i])
	}
	return strings.TrimSpace(eventName)
}

func parseRaceNumber(marketName string) (int, bool) {
	match := raceNumberRe.FindStringSubmatch(marketName)
	if len(match) < 2 {
		return 0, false
	}

	number, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}

	return number, true
}

func parseDistance(marketName string) int {
	match := distanceRe.FindStringSubmatch(marketName)
	if len(match) < 2 {
		return 0
	}

	distance, _ := strconv.Atoi(match[1])
	return distance
}

// toOrderBook converts betfair price levels into the core order book, keeping at
// most ORDER_BOOK_DEPTH levels. Back is ordered high to low, lay low to high.
// Betfair already returns best-first, but we sort to guarantee the ordering.
func toOrderBook(prices []exchange.PriceSize, descending bool) []OrderBook {
	out := make([]OrderBook, 0, len(prices))
	for _, p := range prices {
		out = append(out, OrderBook{Price: p.Price, Size: p.Size})
	}

	sort.Slice(out, func(i, j int) bool {
		if descending {
			return out[i].Price > out[j].Price
		}
		return out[i].Price < out[j].Price
	})

	if len(out) > ORDER_BOOK_DEPTH {
		out = out[:ORDER_BOOK_DEPTH]
	}

	return out
}

func formatEvents(events []Event) string {
	parts := make([]string, 0, len(events))
	for _, event := range events {
		parts = append(parts, fmt.Sprintf("%s %s(%s) %d", event.Country, event.TrackName, event.Code, len(event.Races)))
	}
	return strings.Join(parts, ", ")
}

func countRaces(events []Event) int {
	total := 0
	for _, event := range events {
		total += len(event.Races)
	}
	return total
}
