// packages/core/update.go

package core

import (
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/logger"
)

type RaceStatus int

const (
	StatusUnknown RaceStatus = iota
	StatusPreOff
	StatusRunning
	StatusFinished
)

func (s RaceStatus) String() string {
	switch s {
	case StatusPreOff:
		return "PreOff"
	case StatusRunning:
		return "Running"
	case StatusFinished:
		return "Finished"
	default:
		return "Unknown"
	}
}

type RaceRef struct {
	// unique for race
	Key string

	Scope      string // country/code
	Date       string // meeting date, YYYY-MM-DD
	Venue      string
	VenueName  string // betmatic (if applicable)
	Country    string
	RaceNumber int
	Code       betmatic.RacingCode

	Distance float64

	Status RaceStatus
}

// LogRace returns the race as log lines name it.
func (r RaceRef) LogRace() *logger.Race {
	return &logger.Race{Venue: r.VenueName, Number: r.RaceNumber}
}
