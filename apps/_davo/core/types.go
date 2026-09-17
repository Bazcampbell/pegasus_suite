// core/types.go

package core

type DavoRaceMessage struct {
	Venue        string
	RaceNumber   int
	RunnerNumber int
	RunnerName   string
	UnitSize     float64
	Market       string
	RatedOdds    float64
}
