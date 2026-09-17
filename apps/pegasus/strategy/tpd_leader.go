// strategy/tpd_leader.go

package strategy

import (
	"fmt"
	"strconv"
	"time"

	"racing_wagering/apps/pegasus/core"
	"racing_wagering/apps/pegasus/tpd"

	logger "racing_wagering/logger"
)

// Gmax drop a runner from the running order when its tracking fails, so a short
// order against a full field means the leader is a guess rather than a reading.
// Below this many ranked runners the race is left alone.
const minRankedRunners = 4

type TPDLeader struct {
	raceState map[string]tpdRaceState
}

func NewTPDLeader() Strategy {
	return &TPDLeader{raceState: make(map[string]tpdRaceState)}
}

func (s *TPDLeader) Name() string { return "TPD_Leader" }

func (s *TPDLeader) Code() string { return "tpdl" }

func (s *TPDLeader) Select(u core.Update, betfairDelay, betmaticDelay int64,
	_ func(core.RaceRef) *core.BetfairRace) (core.Decision, error) {

	p, ok := u.Msg.(tpd.Progress)
	if !ok {
		return core.Decision{}, fmt.Errorf("%s: not a tpd progress packet", s.Name())
	}

	key := u.Ref.Key

	if u.Ref.Status == core.StatusFinished {
		delete(s.raceState, key)
		return core.Decision{}, nil
	}

	state := s.raceState[key]
	if state.ignore {
		return core.Decision{}, nil
	}

	if u.Ref.Status != core.StatusRunning {
		s.raceState[key] = state
		return core.Decision{Tracking: true}, nil
	}

	// Gmax's start signal fails at some tracks and leaves the running time at
	// zero for the whole race while everything else stays valid. The first
	// update that reads as running is the fallback clock.
	if state.runningSince.IsZero() {
		state.runningSince = time.Now()
	}
	s.raceState[key] = state

	elapsed := time.Duration(p.RunningTime * float64(time.Second))
	if elapsed <= 0 {
		elapsed = time.Since(state.runningSince)
	}

	if state.betfairPlaced && state.betmaticPlaced {
		return core.Decision{}, nil
	}

	betfairReady := !state.betfairPlaced && elapsed.Milliseconds() >= betfairDelay
	betmaticReady := !state.betmaticPlaced && elapsed.Milliseconds() >= betmaticDelay

	if !betfairReady && !betmaticReady {
		return core.Decision{Tracking: true}, nil
	}

	if reason := scopeBlock(p); reason != "" {
		logger.Warn(logger.ErrorLog{
			Message:     "tpd leader: not betting, " + reason,
			RaceDetails: &logger.RaceDetails{Venue: u.Ref.VenueName, RaceNumber: u.Ref.RaceNumber},
		})
		return core.Decision{Tracking: true}, nil
	}

	leader, err := strconv.Atoi(p.Order[0])
	if err != nil || leader < 1 {
		return core.Decision{Tracking: true}, nil
	}

	unit := s.getUnitSize(u.Ref.Distance)
	if unit <= 0 {
		state.ignore = true
		s.raceState[key] = state
		return core.Decision{}, nil
	}

	decision := core.Decision{Tracking: true}

	if betmaticReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: u.Ref, Side: core.BetmaticWin, Runner: leader, Unit: unit})
		state.betmaticPlaced = true
	}
	if betfairReady {
		decision.Bets = append(decision.Bets, core.Bet{Ref: u.Ref, Side: core.BetfairBack, Runner: leader, Unit: unit})
		state.betfairPlaced = true
	}

	s.raceState[key] = state

	return decision, nil
}

// scopeBlock returns why this packet must not be bet on, or "" when it is safe.
// An assignment warning is the serious one: Gmax are saying the numbers may be
// attached to the wrong horse, which is exactly the wrong time to back one.
func scopeBlock(p tpd.Progress) string {
	if p.Warnings&tpd.WarnAssignment != 0 {
		return "assignment warning set"
	}
	if p.Warnings&tpd.WarnStart != 0 {
		return "start warning set"
	}
	if len(p.Order) < minRankedRunners {
		return fmt.Sprintf("only %d ranked runners", len(p.Order))
	}
	return ""
}

// TODO - decide staking plan
func (s *TPDLeader) getUnitSize(distance float64) float64 {
	return 1
}
