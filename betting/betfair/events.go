// betfair/events.go

package betfair

import (
	"fmt"
	"regexp"

	"pegasus_suite/logger"
)

var (
	raceNumberRe = regexp.MustCompile(`(?i)^R(\d+)`)
	distanceRe   = regexp.MustCompile(`(\d+)m`)

	// The marker is its own word — "R1 2030m Pace", "R4 1609m Trot Final" — so
	// both boundaries are required: "Space" and "Pacemaker" must not match.
	harnessMarketRe = regexp.MustCompile(`(?i)\b(pace|trot)\b`)
)

func (bc *Client) loadUpcomingEvents(countryCodes []string) {
	logger.Debug(logger.InfoLog{Message: "betfair load upcoming events started"})

	eventResults, err := bc.api.ListEvents(listEventsFilter(countryCodes))
	if err != nil {
		logger.Warn(logger.ErrorLog{
			Message: fmt.Sprintf("betfair load upcoming events failed error=%v", err),
		})
		return
	}

	events := make([]Event, 0, len(eventResults))
	for _, er := range eventResults {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("betfair event received event_id=%v event_name=%v venue=%v country=%v open_date=%v market_count=%v", er.Event.ID, er.Event.Name, er.Event.Venue, er.Event.CountryCode, er.Event.OpenDate, er.MarketCount),
		})

		races, err := bc.listRaces(er.Event.ID)
		if err != nil {
			logger.Error(logger.ErrorLog{
				Message: fmt.Sprintf("betfair load races failed event_id=%v event_name=%v error=%v", er.Event.ID, er.Event.Name, err),
			})
			continue
		}

		trackName := parseTrackName(er.Event.Name)
		code := racingCode(races)

		events = append(events, Event{
			Country:   er.Event.CountryCode,
			TrackName: trackName,
			Code:      code,
			Races:     races,
		})

		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("betfair event mapped country=%v track_name=%v event_name=%v code=%v race_count=%v", er.Event.CountryCode, trackName, er.Event.Name, code, len(races)),
		})
	}

	bc.setEvents(events)

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("betfair upcoming events refreshed tracks=%d races=%d detail=[%s]",
			len(events), countRaces(events), formatEvents(events)),
	})
}
