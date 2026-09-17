// packages/tpd/update.go

package tpd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"racing_wagering/apps/pegasus/core"

	logger "racing_wagering/logger"
)

const (
	// Gmax's start detection depends on a third-party signal that is not
	// reliable at every track, and when it fails the running time simply stays
	// at zero while the rest of the packet stays valid. Progress falling below
	// the official distance is the independent check; the margin absorbs the
	// jitter in a stationary field.
	offMargin = 5.0

	raceExpiry = 10 * time.Minute
	sweepEvery = time.Minute

	// A sharecode the race list has never heard of retries on every packet, so
	// complaining about it is rate limited to once per race per this long.
	unresolvedQuiet = 5 * time.Minute
)

// Tracker answers who a progress packet is about, the job RaceMessage.Ref does
// for triple-s. TPD needs memory to do it: the packet carries a sharecode and
// telemetry but no venue and no race state, so identity is resolved once per
// race and status is derived from the distance still to run.
type Tracker struct {
	lookup func(sharecode string) (Race, bool)

	mu         sync.Mutex
	races      map[string]*raceState
	unresolved map[string]time.Time
}

type raceState struct {
	ref core.RaceRef

	// officialDistance is the largest Progress seen, the fallback for a race the
	// list has no length for. Gmax set Progress to the official race distance
	// before the off and it only falls once running, so the maximum is correct
	// even when the feed is joined mid-race.
	officialDistance float64

	finished  bool
	timestamp time.Time
	touchedAt time.Time

	// Last published status, so a transition can be logged without a line per
	// packet at 2Hz.
	lastStatus core.RaceStatus
}

func NewTracker(lookup func(sharecode string) (Race, bool)) *Tracker {
	return &Tracker{
		lookup:     lookup,
		races:      make(map[string]*raceState),
		unresolved: make(map[string]time.Time),
	}
}

// Ref resolves one progress packet. Every packet is already a complete picture
// of the race, so there is nothing to accumulate and nothing to wait for. False
// means the packet is not actionable: an unreadable timestamp, a race that does
// not resolve, or a snapshot older than one already acted on — which is what
// makes a second parallel feed safe to take.
func (t *Tracker) Ref(p Progress) (core.RaceRef, bool) {
	sharecode := string(p.ID)

	timestamp, err := p.Time()
	if err != nil {
		return core.RaceRef{}, false
	}

	t.mu.Lock()

	race, ok := t.races[sharecode]
	if !ok {
		race = t.trackRaceLocked(sharecode)
		if race == nil {
			t.mu.Unlock()
			return core.RaceRef{}, false
		}
	}

	// Reordered and duplicated packets both land here. Neither can be folded in:
	// the newer snapshot has already been acted on.
	if !timestamp.After(race.timestamp) {
		t.mu.Unlock()
		return core.RaceRef{}, false
	}

	race.timestamp = timestamp
	race.touchedAt = time.Now()
	race.officialDistance = max(race.officialDistance, p.Progress)
	race.finished = race.finished || p.Progress <= 0

	ref := race.ref
	ref.Status = race.status(p)
	if ref.Distance == 0 {
		ref.Distance = race.officialDistance
	}

	changed := ref.Status != race.lastStatus
	race.lastStatus = ref.Status

	t.mu.Unlock()

	if changed {
		logger.Debug(logger.InfoLog{
			Message:     fmt.Sprintf("tpd race status=%v sharecode=%v elapsed=%.2fs to_go=%.1f of %.1f order=%v warnings=%v", ref.Status, sharecode, p.RunningTime, p.Progress, ref.Distance, p.Order, p.Warnings),
			RaceDetails: &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
		})
	}

	return ref, true
}

// caller must hold t.mu. A race the list has not loaded yet returns nil and is
// retried on the next packet, which arrives half a second later, so a cold
// cache costs a tick rather than a race.
func (t *Tracker) trackRaceLocked(sharecode string) *raceState {
	course, _, err := ParseSharecode(sharecode)
	if err != nil {
		t.warnUnresolvedLocked(sharecode)
		return nil
	}

	// The race list carries venue, country, race number and length, but never
	// says whether a course runs harness or thoroughbred. RaceType is free text
	// and cannot be trusted for it, so the code comes from our own table.
	code, ok := CourseCodes[course]
	if !ok {
		t.warnUnresolvedLocked(sharecode)
		return nil
	}

	listed, ok := t.lookup(sharecode)
	if !ok {
		t.warnUnresolvedLocked(sharecode)
		return nil
	}

	ref := core.RaceRef{
		Provider:   core.ProviderTPD,
		Key:        sharecode,
		Scope:      core.ScopeKey(listed.Country, code),
		Venue:      course,
		VenueName:  listed.Racecourse,
		Country:    listed.Country,
		RaceNumber: listed.RaceNumber,
		Code:       code,
		Distance:   listed.Length,
	}

	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("tpd tracking race sharecode=%v course=%v scope=%v distance=%.1f", sharecode, ref.Venue, ref.Scope, ref.Distance),
		RaceDetails: &logger.RaceDetails{Venue: ref.VenueName, RaceNumber: ref.RaceNumber},
	})

	race := &raceState{ref: ref, touchedAt: time.Now()}
	t.races[sharecode] = race

	return race
}

// caller must hold t.mu. This is the first thing to check on a quiet feed that
// is otherwise receiving packets: data is arriving but no race is listed.
func (t *Tracker) warnUnresolvedLocked(sharecode string) {
	if last, ok := t.unresolved[sharecode]; ok && time.Since(last) < unresolvedQuiet {
		return
	}
	t.unresolved[sharecode] = time.Now()

	course, scheduledOff, err := ParseSharecode(sharecode)
	if err != nil {
		logger.Warn(logger.ErrorLog{
			Message: fmt.Sprintf("tpd packet carries an unparseable sharecode; dropping sharecode=%v error=%v", sharecode, err),
		})
		return
	}

	_, known := CourseCodes[course]
	logger.Warn(logger.ErrorLog{
		Message: fmt.Sprintf("tpd cannot resolve a race; no bet can be placed for it sharecode=%v course=%v course_known=%v scheduled_off=%v", sharecode, course, known, scheduledOff.Format("2006-01-02 15:04")),
	})
}

// caller must hold t.mu.
func (r *raceState) status(p Progress) core.RaceStatus {
	distance := r.ref.Distance
	if distance == 0 {
		distance = r.officialDistance
	}

	switch {
	case r.finished:
		return core.StatusFinished
	case p.RunningTime > 0:
		return core.StatusRunning
	case distance > 0 && p.Progress < distance-offMargin:
		return core.StatusRunning
	default:
		return core.StatusPreOff
	}
}

// SweepExpired drops races the feed has stopped sending. Without it the race
// map grows for the life of the process. It blocks; the tracker itself is
// driven by Ref, not by this.
func (t *Tracker) SweepExpired(ctx context.Context) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			t.mu.Lock()
			for sharecode, race := range t.races {
				if now.Sub(race.touchedAt) > raceExpiry {
					delete(t.races, sharecode)
					delete(t.unresolved, sharecode)
				}
			}
			t.mu.Unlock()
		}
	}
}
