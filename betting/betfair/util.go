// betfair/util.go

package betfair

import (
	"fmt"
	"strconv"
	"strings"

	"pegasus_suite/betting/betfair/internal/exchange"
)

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

// betfair-specific type to common type
func toOrderBook(prices []exchange.PriceSize) []OrderBook {
	out := make([]OrderBook, 0, len(prices))
	for _, p := range prices {
		out = append(out, OrderBook{Price: p.Price, Size: p.Size})
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

func runnerKey(code RacingCode, country, normTrack string, raceNumber int) string {
	return fmt.Sprintf("%s:%s:%s:%d", code, strings.ToUpper(country), normTrack, raceNumber)
}
