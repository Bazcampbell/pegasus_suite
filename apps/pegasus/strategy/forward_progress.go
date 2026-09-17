// strategy/forward_progress.go

package strategy

import (
	"fmt"
	"math"

	"racing_wagering/apps/pegasus/core"
	triples "racing_wagering/apps/pegasus/triples"
	"racing_wagering/apps/pegasus/util"

	logger "racing_wagering/logger"
)

type forwardProgress struct {
	name        string
	code        string
	maxDistance int
	unitFor     func(trackLength int) float64

	raceState map[string]forwardProgressState
}

func NewForwardProgressThoroughbred() Strategy {
	return &forwardProgress{
		name:        "DST_Forward_Progress_Thoroughbred",
		code:        "fpth",
		maxDistance: 1450,
		unitFor: func(trackLength int) float64 {
			switch {
			case trackLength < 1050:
				return 1
			case trackLength < 1250:
				return 0.8
			case trackLength < 1450:
				return 0.6
			}
			return 0
		},
		raceState: make(map[string]forwardProgressState),
	}
}

// Harness races any distance, which is why maxDistance is zero here.
func NewForwardProgressHarness() Strategy {
	return &forwardProgress{
		name:        "DST_Forward_Progress_Harness",
		code:        "fphr",
		maxDistance: 0,
		unitFor: func(trackLength int) float64 {
			if trackLength < 2000 {
				return 1
			}
			return 0.8
		},
		raceState: make(map[string]forwardProgressState),
	}
}

func (s *forwardProgress) Name() string { return s.name }

func (s *forwardProgress) Code() string { return s.code }

func (s *forwardProgress) Select(u core.Update, betfairDelay, betmaticDelay int64,
	getBetfairRace func(core.RaceRef) *core.BetfairRace) (core.Decision, error) {

	race, ok := u.Msg.(triples.RaceMessage)
	if !ok {
		return core.Decision{}, fmt.Errorf("%s: not a triple-s message", s.name)
	}

	key := u.Ref.Key
	state, tracked := s.raceState[key]

	// Handled before anything else so an ignored race is dropped from the map
	// too, rather than being kept for the life of the process.
	if u.Ref.Status == core.StatusFinished {
		delete(s.raceState, key)
		return core.Decision{}, nil
	}

	if !tracked {
		state, ok = s.openRace(u.Ref, getBetfairRace)
		s.raceState[key] = state
		if !ok {
			return core.Decision{}, nil
		}
	}

	if state.ignore {
		return core.Decision{}, nil
	}

	if !state.hasInitial {
		if u.Ref.Status != core.StatusRunning {
			return core.Decision{Tracking: true}, nil
		}

		state.initial = race
		state.hasInitial = true
		s.raceState[key] = state

		logger.Debug(logger.InfoLog{
			Message:     "initial running message received",
			RaceDetails: &logger.RaceDetails{Venue: u.Ref.VenueName, RaceNumber: u.Ref.RaceNumber},
		})
		return core.Decision{Tracking: true}, nil
	}

	if state.betfairPlaced && state.betmaticPlaced {
		return core.Decision{}, nil
	}

	initialAt, err := util.ISO8601ToTime(state.initial.Timestamp)
	if err != nil {
		return core.Decision{Tracking: true}, fmt.Errorf("unable to parse first message timestamp %s: %w", state.initial.Timestamp, err)
	}

	currentAt, err := util.ISO8601ToTime(race.Timestamp)
	if err != nil {
		return core.Decision{Tracking: true}, fmt.Errorf("unable to parse current message timestamp %s: %w", race.Timestamp, err)
	}

	delta := currentAt.Sub(initialAt).Milliseconds()

	betfairReady := !state.betfairPlaced && delta >= betfairDelay
	betmaticReady := !state.betmaticPlaced && delta >= betmaticDelay

	// wait for more information to be received from triples
	if !betfairReady && !betmaticReady {
		return core.Decision{Tracking: true}, nil
	}

	best, worst, ok := forwardProgressExtremes(state.initial, race)
	if !ok {
		return core.Decision{Tracking: true}, nil
	}

	unit := s.unitFor(state.distance)
	if unit <= 0 {
		state.ignore = true
		s.raceState[key] = state
		return core.Decision{}, nil
	}

	decision := core.Decision{Tracking: true}

	if betmaticReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: u.Ref, Side: core.BetmaticWin, Runner: best, Unit: unit})
		state.betmaticPlaced = true
	}

	if betfairReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: u.Ref, Side: core.BetfairBack, Runner: best, Unit: unit})

		if worst != best {
			decision.Bets = append(decision.Bets, core.Bet{Ref: u.Ref, Side: core.BetfairLay, Runner: worst, Unit: unit})
		}
		state.betfairPlaced = true
	}

	s.raceState[key] = state

	return decision, nil
}

// openRace resolves the distance the unit scales on. Triple-S does not carry it,
// so it comes from Betfair, and a race we cannot price is one we cannot size.
func (s *forwardProgress) openRace(ref core.RaceRef, getBetfairRace func(core.RaceRef) *core.BetfairRace) (forwardProgressState, bool) {
	bfRace := getBetfairRace(ref)
	if bfRace == nil {
		logger.Error(logger.ErrorLog{
			Message:     "unable to resolve betfair race",
			RaceDetails: &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
		})
		return forwardProgressState{ignore: true}, false
	}

	if bfRace.Distance < 1 {
		logger.Warn(logger.ErrorLog{
			Message:     fmt.Sprintf("race distance is 0, unable to get race distance: %s", bfRace.Name),
			RaceDetails: &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
		})
		return forwardProgressState{ignore: true}, false
	}

	if s.maxDistance > 0 && bfRace.Distance > s.maxDistance {
		return forwardProgressState{ignore: true}, false
	}

	return forwardProgressState{distance: bfRace.Distance}, true
}

// forwardProgressExtremes returns the saddlecloth numbers that have advanced
// furthest and least along the field's average direction of travel.
func forwardProgressExtremes(start, current triples.RaceMessage) (best, worst int, ok bool) {
	startMap := make(map[int]triples.LiveDataSet, len(start.LiveDataSets))
	for _, h := range start.LiveDataSets {
		startMap[int(h.SaddleclothNumber)] = h
	}

	displacements := make(map[int]core.Vector, len(current.LiveDataSets))

	var fieldVector core.Vector
	var count float64

	for _, curr := range current.LiveDataSets {
		startHorse, seen := startMap[int(curr.SaddleclothNumber)]
		if !seen {
			continue
		}

		d := curr.GPSCoordinates.Sub(startHorse.GPSCoordinates)

		displacements[int(curr.SaddleclothNumber)] = d
		fieldVector = fieldVector.Add(d)
		count++
	}

	if count == 0 {
		return 0, 0, false
	}

	forward := fieldVector.Div(count).Normalise()

	bestProgress := math.Inf(-1)
	worstProgress := math.Inf(1)

	for saddlecloth, d := range displacements {
		progress := d.Dot(forward)
		if progress > bestProgress {
			bestProgress, best = progress, saddlecloth
		}
		if progress < worstProgress {
			worstProgress, worst = progress, saddlecloth
		}
	}

	// cloth can never be sub 1
	if best < 1 && worst < 1 {
		return 0, 0, false
	}

	return best, worst, true
}
