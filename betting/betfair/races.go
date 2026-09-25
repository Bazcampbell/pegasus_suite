// betfair/races.go

package betfair

import (
	"sort"
	"strings"
	"time"

	"pegasus_suite/betting"
)

const (
	// the stream carries prices for thoroughbred races starting in this window
	streamFrom = -1 * time.Hour
	streamTo   = 4 * time.Hour
	// the stream refuses a subscription over 200 markets
	maxStreamMarkets = 200
)

// GetRace returns the race for code, country, track and race number, or nil when it is not loaded.
func (bc *Client) GetRace(code RacingCode, country, trackName string, raceNumber int) *Race {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	events := bc.thoroughbred
	if code == TROT {
		events = bc.trot
	}
	event, ok := events[eventKey(country, trackName)]
	if !ok {
		return nil
	}
	return event.Races[raceNumber]
}

func eventKey(country, trackName string) string {
	return strings.ToUpper(country) + ":" + betting.NormaliseTrackKey(trackName)
}

// setEvents replaces the catalogue and points the stream at its thoroughbred races near their start.
func (bc *Client) setEvents(events []Event) {
	thoroughbred := make(map[string]*Event, len(events))
	trot := make(map[string]*Event, len(events))
	var near []*Race

	now := time.Now()
	for i := range events {
		event := &events[i]
		key := eventKey(event.Country, event.TrackName)
		if event.Code == TROT {
			trot[key] = event
			continue
		}
		thoroughbred[key] = event
		for _, race := range event.Races {
			if race.StartTime.After(now.Add(streamFrom)) && race.StartTime.Before(now.Add(streamTo)) {
				near = append(near, race)
			}
		}
	}

	bc.mu.Lock()
	bc.thoroughbred = thoroughbred
	bc.trot = trot
	bc.mu.Unlock()

	sort.Slice(near, func(i, j int) bool { return near[i].StartTime.Before(near[j].StartTime) })
	near = near[:min(len(near), maxStreamMarkets)]
	ids := make([]string, len(near))
	for i, race := range near {
		ids[i] = race.ID
	}
	bc.subscribe(ids)
}
