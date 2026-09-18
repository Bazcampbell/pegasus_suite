// packages/betting/resolve.go

package betting

import (
	"pegasus_suite/apps/davo/util"
	"errors"
	"fmt"
	"pegasus_suite/betting/betmatic"
	logger "pegasus_suite/logger"
	"sort"
	"strconv"
	"strings"
)

const (
	nameMatchFloor = 0.45

	// 1st has to beat 2nd by this much
	nameMatchMargin = 0.15

	// added to candidate whose saddlecloth number matches the tip
	numberMatchBonus = 0.35
)

// race number not in map at all
var ErrNoCandidates = errors.New("no candidates for race")

// finds runner a tip refers to
// every runner in the race is considered across all venues + race number
// best match wins
func ResolveEvent(eventMap map[int]map[string]betmatic.Event, raceNumber, runnerNumber int, runnerName string) (EventMatch, error) {
	candidates, err := rank(eventMap, raceNumber, runnerNumber, runnerName)
	if err != nil {
		return EventMatch{}, err
	}

	best := candidates[0]

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("local resolve: ranked field needle=%v candidates=%v best=%v best_venue=%v best_number=%v best_name_score=%.3f best_total_score=%.3f",
			runnerName, len(candidates), best.RunnerName, best.Venue, best.RunnerNo, best.nameScore, best.score),
		RaceDetails: &logger.RaceDetails{RaceNumber: raceNumber},
	})

	if len(candidates) > 1 {
		r := candidates[1]
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("local resolve: runner-up runner=%v number=%v total_score=%.3f margin=%.3f margin_required=%v",
				r.RunnerName, r.RunnerNo, r.score, best.score-r.score, nameMatchMargin),
			RaceDetails: &logger.RaceDetails{Venue: r.Venue},
		})
	}

	if best.nameScore < nameMatchFloor {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("local resolve: below name floor score=%.3f floor=%v", best.nameScore, nameMatchFloor),
		})
		return EventMatch{}, fmt.Errorf("no runner in R%d resembles %q (closest %q #%d at %.0f%%, need %.0f%%)",
			raceNumber, runnerName, best.RunnerName, best.RunnerNo, best.nameScore*100, nameMatchFloor*100)
	}

	// only a close second is a problem
	// anything further back is just another runner in the race.
	if len(candidates) > 1 {
		second := candidates[1]
		if best.score-second.score < nameMatchMargin {
			logger.Debug(logger.InfoLog{
				Message: "local resolve: ambiguous, escalating",
			})
			return EventMatch{}, fmt.Errorf("ambiguous runner for R%d %q: %q #%d at %s and %q #%d at %s score too close (%.2f vs %.2f)",
				raceNumber, runnerName,
				best.RunnerName, best.RunnerNo, best.Venue,
				second.RunnerName, second.RunnerNo, second.Venue,
				best.score, second.score)
		}
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("local resolve: matched runner=%v number=%v score=%.3f",
			best.RunnerName, best.RunnerNo, best.nameScore),
		RaceDetails: &logger.RaceDetails{Venue: best.Venue},
	})

	return EventMatch{
		Venue:      best.Venue,
		RunnerName: best.RunnerName,
		RunnerNo:   best.RunnerNo,
		Score:      best.nameScore,
	}, nil
}

// scores every runner in the race and returns them best > first
func rank(eventMap map[int]map[string]betmatic.Event, raceNumber, runnerNumber int, runnerName string) ([]scored, error) {
	if eventMap == nil {
		return nil, fmt.Errorf("event map not loaded: %w", ErrNoCandidates)
	}

	events, ok := eventMap[raceNumber]
	if !ok {
		return nil, fmt.Errorf("no events found for R%d: %w", raceNumber, ErrNoCandidates)
	}

	needle := util.Normalise(runnerName)
	candidates := make([]scored, 0)

	for runnerNames, event := range events {
		names := strings.Split(runnerNames, "#&#")
		numbers := strings.Split(event.Runners, ",")

		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}

			// Runner numbers are a parallel list to runner names. A runner with no
			// number can still be matched on name alone, it just cannot earn the
			// number bonus.
			num := 0
			if i < len(numbers) {
				if parsed, err := strconv.Atoi(strings.TrimSpace(numbers[i])); err == nil {
					num = parsed
				}
			}

			c := scored{
				Candidate: Candidate{
					RaceNumber: raceNumber,
					Venue:      event.Name,
					RunnerName: name,
					RunnerNo:   num,
				},
				nameScore: similarity(util.Normalise(name), needle),
			}

			c.score = c.nameScore
			if num != 0 && num == runnerNumber {
				c.score += numberMatchBonus
			}

			c.Candidate.Score = c.nameScore
			candidates = append(candidates, c)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no runners listed for R%d: %w", raceNumber, ErrNoCandidates)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return candidates, nil
}

// 0-1
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}

	dist := util.Levenshtein(a, b)
	longest := len(a)
	if len(b) > longest {
		longest = len(b)
	}

	return 1 - float64(dist)/float64(longest)
}
