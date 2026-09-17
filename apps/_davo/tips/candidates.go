// packages/betting/candidates.go

package betting

import (
	"fmt"
	"racing_wagering/betting/betmatic"
	"sort"
	"strconv"
	"strings"
)

func RaceCandidates(eventMap map[int]map[string]betmatic.Event, raceNumber int) ([]Candidate, error) {
	if eventMap == nil {
		return nil, fmt.Errorf("event map not loaded: %w", ErrNoCandidates)
	}

	events, ok := eventMap[raceNumber]
	if !ok {
		return nil, fmt.Errorf("no events found for R%d: %w", raceNumber, ErrNoCandidates)
	}

	out := collect(events, raceNumber)
	if len(out) == 0 {
		return nil, fmt.Errorf("no runners listed for R%d: %w", raceNumber, ErrNoCandidates)
	}

	sortCandidates(out)
	return out, nil
}

// returns every runner in every upcoming race
func AllCandidates(eventMap map[int]map[string]betmatic.Event) ([]Candidate, error) {
	if eventMap == nil {
		return nil, fmt.Errorf("event map not loaded: %w", ErrNoCandidates)
	}

	out := make([]Candidate, 0)
	for raceNumber, events := range eventMap {
		out = append(out, collect(events, raceNumber)...)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no runners in event map: %w", ErrNoCandidates)
	}

	sortCandidates(out)
	return out, nil
}

// flattens one race number's events into candidates. names and
// numbers are parallel lists on the event
// runner missing a number is still included as name stronger identifier
func collect(events map[string]betmatic.Event, raceNumber int) []Candidate {
	out := make([]Candidate, 0)

	for runnerNames, event := range events {
		names := strings.Split(runnerNames, "#&#")
		numbers := strings.Split(event.Runners, ",")

		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}

			num := 0
			if i < len(numbers) {
				if parsed, err := strconv.Atoi(strings.TrimSpace(numbers[i])); err == nil {
					num = parsed
				}
			}

			out = append(out, Candidate{
				RaceNumber: raceNumber,
				Venue:      event.Name,
				RunnerName: name,
				RunnerNo:   num,
			})
		}
	}

	return out
}

// prompt caching + logging readability
func sortCandidates(c []Candidate) {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].RaceNumber != c[j].RaceNumber {
			return c[i].RaceNumber < c[j].RaceNumber
		}
		if c[i].Venue != c[j].Venue {
			return c[i].Venue < c[j].Venue
		}
		return c[i].RunnerNo < c[j].RunnerNo
	})
}
