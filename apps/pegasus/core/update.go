// packages/core/update.go

package core

import "pegasus_suite/betting/betmatic"

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
	Venue      string
	VenueName  string // betmatic (if applicable)
	Country    string
	RaceNumber int
	Code       betmatic.RacingCode

	Distance float64

	Status RaceStatus
}
