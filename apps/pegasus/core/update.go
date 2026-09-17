// packages/core/update.go

package core

import "racing_wagering/betting/betmatic"

type Provider string

const (
	ProviderTripleS Provider = "triple-s"
	ProviderTPD     Provider = "tpd"
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
	Provider Provider

	// unique for race
	Key string

	Scope      string // country/code
	Venue      string
	VenueName  string
	Country    string
	RaceNumber int
	Code       betmatic.RacingCode

	Distance float64

	Status RaceStatus
}

// engine fans this out
// msg is the actual payload of the stream
type Update struct {
	Ref RaceRef
	Msg any
}
