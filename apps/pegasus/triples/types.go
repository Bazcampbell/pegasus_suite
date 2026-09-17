// packages/triple-s/types.go

package triples

import (
	"racing_wagering/apps/pegasus/core"
)

type RaceState int

const (
	RaceReset       RaceState = 255
	RaceInitialised RaceState = 0 // imported
	RaceScheduled   RaceState = 1 // about 15 minutes before jump
	RaceReady       RaceState = 2 // about 5 minutes before jump
	RaceArmed       RaceState = 3 // loading/starting preperation
	RaceRunning     RaceState = 4
	RaceFinished    RaceState = 5
	FalseStart      RaceState = 6
)

func (r RaceState) String() string {
	switch r {
	case RaceReset:
		return "RaceReset"
	case RaceInitialised:
		return "RaceInitialised"
	case RaceScheduled:
		return "RaceScheduled"
	case RaceReady:
		return "RaceReady"
	case RaceArmed:
		return "RaceArmed"
	case RaceRunning:
		return "RaceRunning"
	case RaceFinished:
		return "RaceFinished"
	case FalseStart:
		return "FalseStart"
	default:
		return "UnknownRaceState"
	}
}

type ResultState int

const (
	Competes     ResultState = 0 // will start
	Running      ResultState = 1
	DidNotStart  ResultState = 2 // scratched before race
	DidNotFinish ResultState = 3 // out during race
	Finished     ResultState = 4
	DidNotTrack  ResultState = 5 // no data captured
	Disqualified ResultState = 6
)

func (r ResultState) String() string {
	switch r {
	case Competes:
		return "Competes"
	case Running:
		return "Running"
	case DidNotStart:
		return "DidNotStart"
	case DidNotFinish:
		return "DidNotFinish"
	case Finished:
		return "Finished"
	case DidNotTrack:
		return "DidNotTrack"
	case Disqualified:
		return "Disqualified"
	default:
		return "UnknownResultState"
	}
}

type RaceMessage struct {
	Topic           string
	CountryRaceCode string // COUNTRY/CODE like AUS/RVIC

	Timestamp    string        `json:"timestamp"` // ISO 8601 UTC
	Venue        Venue         `json:"venue"`
	EventDate    string        `json:"event_date"` // YYYY-MM-DD
	RaceNumber   int           `json:"race_number"`
	RaceState    RaceState     `json:"race_state"`
	LiveDataSets []LiveDataSet `json:"live_data_sets"`

	// The feed also carries a field_sectional_data_set per message. Nothing
	// reads it, so it is left undecoded rather than allocated and discarded.
}

type Venue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LiveDataSet struct {
	SaddleclothNumber                 core.FlexInt `json:"saddlecloth_number"`
	ResultState                       ResultState  `json:"result_state"` // 0-6
	CurrentRank                       core.FlexInt `json:"current_rank"`
	CurrentRankIndex                  core.FlexInt `json:"current_rank_index"`
	DistanceToGo                      core.FlexInt `json:"distance_to_go"`                          // meters; "NaN" pre-race
	DistanceToGoInRankUpdateFrequency core.FlexInt `json:"distance_to_go_in_rank_update_frequency"` // meters; may be float on the wire
	NormalizedDistanceToRail          float64      `json:"normalized_distance_to_rail"`             // 0-1
	GPSCoordinates                    core.Vector  `json:"gps_coordinates"`
	Speed                             float64      `json:"speed"`         // m/s
	StrideRate                        float64      `json:"stride_rate"`   // Hz
	StrideLength                      float64      `json:"stride_length"` // meters
	DistanceRun                       float64      `json:"distance_run"`  // meters
}

// FlexInt fields above use core.FlexInt, which accepts the loosely-typed shapes
// the triple-s feed emits (bare ints, floats, stringified numbers, "" and "NaN"
// → 0). See packages/core/flexnum.go.
