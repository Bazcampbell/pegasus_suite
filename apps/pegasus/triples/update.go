// packages/triple-s/update.go

package triples

import (
	"fmt"

	"pegasus_suite/apps/pegasus/core"
)

// Triple-S covers Australia only. The scope grid keys on country, so this is
// what every message from this feed resolves to.
const country = "AU"

// Ref resolves who a message is about. It carries nothing about the race in
// flight: the GPS, the ranks and the result states stay on RaceMessage, and
// only ForwardProgress reads them.
//
// Returns false for a topic we have no racing code for, and for the race states
// no strategy acts on, so those never reach a process at all.
func (m RaceMessage) Ref() (core.RaceRef, bool) {
	code, ok := core.TriplesRacingCodes[m.CountryRaceCode]
	if !ok {
		return core.RaceRef{}, false
	}

	status := m.RaceState.status()
	if status == core.StatusUnknown {
		return core.RaceRef{}, false
	}

	return core.RaceRef{
		Provider: core.ProviderTripleS,
		// The event date is in the key because a venue runs the same race
		// numbers again tomorrow, and strategy state is keyed on this.
		Key:        fmt.Sprintf("triple-s/%s/%s/%d", m.EventDate, m.Venue.Name, m.RaceNumber),
		Scope:      core.ScopeKey(country, code),
		Venue:      m.Venue.Name,
		VenueName:  m.Venue.Name,
		Country:    country,
		RaceNumber: m.RaceNumber,
		Code:       code,
		Status:     status,
	}, true
}

func (r RaceState) status() core.RaceStatus {
	switch r {
	case RaceRunning:
		return core.StatusRunning
	case RaceFinished:
		return core.StatusFinished
	case RaceInitialised, RaceScheduled, RaceReady, RaceArmed:
		return core.StatusPreOff
	default:
		return core.StatusUnknown
	}
}
