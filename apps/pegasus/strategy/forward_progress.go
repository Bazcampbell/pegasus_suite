// strategy/forward_progress.go

package strategy

import (
	"fmt"
	"math"
	"time"

	"pegasus_suite/apps/pegasus/core"
	triples "pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/apps/pegasus/util"
	"pegasus_suite/betting/betmatic"

	logger "pegasus_suite/logger"
)

type ForwardProgress struct {
	races map[string]forwardProgressState
	moves []runnerMove
}

type runnerMove struct {
	cloth int
	d     core.Vector
}

func NewForwardProgress() *ForwardProgress {
	return &ForwardProgress{races: make(map[string]forwardProgressState)}
}

func (s *ForwardProgress) Select(m triples.RaceMessage, ref core.RaceRef, betfairDelay, betmaticDelay int64,
	getBetfairRace func(core.RaceRef) *core.BetfairRace) (core.Decision, error) {

	key := ref.Key
	state, tracked := s.races[key]

	// Handled before anything else so an ignored race is dropped from the map
	// too, rather than being kept for the life of the process.
	if ref.Status == core.StatusFinished {
		delete(s.races, key)
		return core.Decision{}, nil
	}

	if !tracked {
		var ok bool
		state, ok = openRace(ref, getBetfairRace)
		s.races[key] = state
		if !ok {
			return core.Decision{}, nil
		}
	}

	if state.ignore {
		return core.Decision{}, nil
	}

	if !state.started {
		if ref.Status != core.StatusRunning {
			return core.Decision{Tracking: true}, nil
		}

		at, err := util.ISO8601ToTime(m.Timestamp)
		if err != nil {
			return core.Decision{Tracking: true}, fmt.Errorf("unable to parse first running message timestamp %s: %w", m.Timestamp, err)
		}

		state.started = true
		state.startedAt = at
		state.start = make(map[int]core.Vector, len(m.LiveDataSets))
		for _, h := range m.LiveDataSets {
			state.start[int(h.SaddleclothNumber)] = h.GPSCoordinates
		}
		s.races[key] = state

		logger.Debug(logger.Log{
			App:     core.AppName,
			Message: "initial running message received",
			Race:    &logger.Race{Venue: ref.VenueName, Number: ref.RaceNumber},
		})
		return core.Decision{Tracking: true}, nil
	}

	if state.betfairPlaced && state.betmaticPlaced {
		return core.Decision{}, nil
	}

	at, err := util.ISO8601ToTime(m.Timestamp)
	if err != nil {
		return core.Decision{Tracking: true}, fmt.Errorf("unable to parse current message timestamp %s: %w", m.Timestamp, err)
	}

	delta := at.Sub(state.startedAt).Milliseconds()

	betfairReady := !state.betfairPlaced && delta >= betfairDelay
	betmaticReady := !state.betmaticPlaced && delta >= betmaticDelay

	// wait for more information to be received from triples
	if !betfairReady && !betmaticReady {
		return core.Decision{Tracking: true}, nil
	}

	best, worst, ok := s.extremes(state.start, m.LiveDataSets)
	if !ok {
		return core.Decision{Tracking: true}, nil
	}

	unit := unitFor(ref.Code, state.distance)
	if unit <= 0 {
		state.ignore = true
		s.races[key] = state
		return core.Decision{}, nil
	}

	decision := core.Decision{Tracking: true}

	if betmaticReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: ref, Side: core.BetmaticWin, Runner: best, Unit: unit})
		state.betmaticPlaced = true
	}

	if betfairReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: ref, Side: core.BetfairBack, Runner: best, Unit: unit})

		if worst != best {
			decision.Bets = append(decision.Bets, core.Bet{Ref: ref, Side: core.BetfairLay, Runner: worst, Unit: unit})
		}
		state.betfairPlaced = true
	}

	s.races[key] = state

	return decision, nil
}

// openRace resolves the distance the unit scales on. Triple-S does not carry it,
// so it comes from Betfair, and a race we cannot price is one we cannot size.
func openRace(ref core.RaceRef, getBetfairRace func(core.RaceRef) *core.BetfairRace) (forwardProgressState, bool) {
	if ref.Code != betmatic.THOROUGHBRED && ref.Code != betmatic.HARNESS {
		logger.Warn(logger.Log{
			App:     core.AppName,
			Message: fmt.Sprintf("forward progress does not bet racing code %q", ref.Code),
			Race:    &logger.Race{Venue: ref.VenueName, Number: ref.RaceNumber},
		})
		return forwardProgressState{ignore: true}, false
	}

	bfRace := getBetfairRace(ref)
	if bfRace == nil {
		logger.Error(logger.Log{
			App:     core.AppName,
			Message: "unable to resolve betfair race",
			Race:    &logger.Race{Venue: ref.VenueName, Number: ref.RaceNumber},
		})
		return forwardProgressState{ignore: true}, false
	}

	if bfRace.Distance < 1 {
		logger.Warn(logger.Log{
			App:     core.AppName,
			Message: fmt.Sprintf("race distance is 0, unable to get race distance: %s", bfRace.Name),
			Race:    &logger.Race{Venue: ref.VenueName, Number: ref.RaceNumber},
		})
		return forwardProgressState{ignore: true}, false
	}

	// Harness races any distance.
	if ref.Code == betmatic.THOROUGHBRED && bfRace.Distance > 1450 {
		return forwardProgressState{ignore: true}, false
	}

	return forwardProgressState{distance: bfRace.Distance}, true
}

func unitFor(code betmatic.RacingCode, trackLength int) float64 {
	switch code {
	case betmatic.THOROUGHBRED:
		switch {
		case trackLength < 1050:
			return 1
		case trackLength < 1250:
			return 0.8
		case trackLength < 1450:
			return 0.6
		}
	case betmatic.HARNESS:
		if trackLength < 2000 {
			return 1
		}
		return 0.8
	}
	return 0
}

// extremes returns the saddlecloth numbers that have advanced furthest and
// least along the field's average direction of travel.
func (s *ForwardProgress) extremes(start map[int]core.Vector, current []triples.LiveDataSet) (best, worst int, ok bool) {
	moves := s.moves[:0]
	var fieldVector core.Vector

	for _, h := range current {
		cloth := int(h.SaddleclothNumber)
		from, seen := start[cloth]
		if !seen {
			continue
		}
		d := h.GPSCoordinates.Sub(from)
		moves = append(moves, runnerMove{cloth: cloth, d: d})
		fieldVector = fieldVector.Add(d)
	}
	s.moves = moves

	if len(moves) == 0 {
		return 0, 0, false
	}

	forward := fieldVector.Div(float64(len(moves))).Normalise()

	bestProgress := math.Inf(-1)
	worstProgress := math.Inf(1)

	for _, mv := range moves {
		progress := mv.d.Dot(forward)
		if progress > bestProgress {
			bestProgress, best = progress, mv.cloth
		}
		if progress < worstProgress {
			worstProgress, worst = progress, mv.cloth
		}
	}

	// cloth can never be sub 1
	if best < 1 && worst < 1 {
		return 0, 0, false
	}

	return best, worst, true
}

type forwardProgressState struct {
	ignore         bool
	started        bool
	betfairPlaced  bool
	betmaticPlaced bool
	distance       int
	startedAt      time.Time
	start          map[int]core.Vector
}
