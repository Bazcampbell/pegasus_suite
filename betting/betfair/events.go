// betfair/events.go

package betfair

import (
	"fmt"
	"regexp"

	"pegasus_suite/logger"
)

var (
	raceNumberRe    = regexp.MustCompile(`(?i)^R(\d+)`)
	distanceRe      = regexp.MustCompile(`(\d+)m`)
	harnessMarketRe = regexp.MustCompile(`(?i)\b(pace|trot)\b`)
)

// loadUpcomingEvents reloads the horse racing catalogue for countryCodes; on failure the old one stays.
func (bc *Client) loadUpcomingEvents(countryCodes []string) {
	results, err := bc.api.ListEvents(listEventsFilter(countryCodes))
	if err != nil {
		logger.Warn(logger.Log{Message: fmt.Sprintf("betfair load upcoming events failed error=%v", err)})
		return
	}

	events := make([]Event, 0, len(results))
	races := 0
	for _, r := range results {
		eventRaces, err := bc.listRaces(r.Event.ID)
		if err != nil {
			logger.Error(logger.Log{Message: fmt.Sprintf("betfair load races failed event=%v error=%v", r.Event.Name, err)})
			continue
		}
		events = append(events, Event{
			Country:   r.Event.CountryCode,
			TrackName: parseTrackName(r.Event.Name),
			Code:      racingCode(eventRaces),
			Races:     eventRaces,
		})
		races += len(eventRaces)
	}

	bc.setEvents(events)
	logger.Debug(logger.Log{Message: fmt.Sprintf("betfair catalogue refreshed tracks=%d races=%d", len(events), races)})
}
