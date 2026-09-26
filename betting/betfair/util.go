// betfair/util.go

package betfair

import (
	"strconv"
	"strings"
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
	return number, err == nil
}

func parseDistance(marketName string) int {
	match := distanceRe.FindStringSubmatch(marketName)
	if len(match) < 2 {
		return 0
	}
	distance, _ := strconv.Atoi(match[1])
	return distance
}
