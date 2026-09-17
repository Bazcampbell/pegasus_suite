// packages/tpd/types.go

package tpd

import (
	"bytes"
	"fmt"
	"time"
)

// tpd has two stream types, progress and gps
const progressPacketKind = 5

// race ID
const sharecodeLen = 14

// operator warning bits, GX-UG-00020
// bits 0, 1, 5, 6 7 are reserved
// WarnAssignment means the numbers may be attached to wrong horse
const (
	WarnStart      uint32 = 1 << 2
	WarnAssignment uint32 = 1 << 3
	WarnField      uint32 = 1 << 4
)

// GX-UG-00020, one live race progress msg
// ID is the bare race identifier
type Progress struct {
	Kind      int    `json:"K"`
	Timestamp string `json:"T"`
	ID        GmaxID `json:"I"`

	// last timing gate to record a sectional
	Gate string `json:"G"`

	// gate's distance from finish
	GateLength float64 `json:"L"`

	SectionalTime  float64 `json:"S"` // seconds, previous gate to this one, leaders
	CumulativeTime float64 `json:"C"` // seconds, start to this gate, leaders
	RunningTime    float64 `json:"R"` // seconds since the start of the race
	LeaderSpeed    float64 `json:"V"` // m/s

	Progress float64 `json:"P"` // official metres remaining to the finish

	Order        []string  `json:"O"` // saddlecloths, leader first
	Field        []string  `json:"F"` // saddlecloths considered to be running
	DistanceBack []float64 `json:"B"` // metres behind the leader, indexed like Order

	// Gmax describes bits 0 to 7 but tells subscribers to tolerate the feed
	// being extended. A width that cannot hold a wider bit field would fail the
	// whole packet, and a widened warning flag is not a reason to go blind.
	Warnings uint32 `json:"W"`
}

func (p Progress) Time() (time.Time, error) {
	return time.Parse(time.RFC3339, p.Timestamp)
}

type Race struct {
	ID         GmaxID    `json:"I"`
	Country    string    `json:"Country"`
	Racecourse string    `json:"Racecourse"`
	RaceNumber int       `json:"RaceNo"`
	PostTime   time.Time `json:"PostTime"`
	RaceType   string    `json:"RaceType"`
	Length     float64   `json:"RaceLength"`
	Published  bool      `json:"Published"`

	EQBRacecourse  string `json:"EQBRacecourse"`
	USTARacecourse string `json:"USTARacecourse"`
}

type GmaxID string

func (i *GmaxID) UnmarshalJSON(b []byte) error {
	*i = GmaxID(bytes.Trim(b, `"`))
	return nil
}

func ParseSharecode(s string) (course string, scheduledOff time.Time, err error) {
	if len(s) != sharecodeLen {
		return "", time.Time{}, fmt.Errorf("tpd: bad sharecode %q", s)
	}

	scheduledOff, err = time.Parse("200601021504", s[2:])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("tpd: bad sharecode time %q: %w", s, err)
	}

	return s[:2], scheduledOff, nil
}
