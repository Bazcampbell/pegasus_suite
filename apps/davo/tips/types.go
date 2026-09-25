// davo/tips/types.go

package tips

import "fmt"

type Candidate struct {
	Date       string // race date, YYYY-MM-DD; "" when Betmatic gave none
	RaceNumber int
	Venue      string
	RunnerName string
	RunnerNo   int
	Score      float64
}

type scored struct {
	Candidate
	nameScore float64 // name similarity alone, 0..1
	score     float64 // nameScore plus the number-match bonus
}

type DavoBet struct {
	RaceNumber int
	RunnerNo   int
	RunnerName string
	Venue      string
	Stake      float64 // in units
	Market     string  // "win" or "place"
	RatedOdds  float64 // rated price from tipster, 0 if not provided
}

func (b DavoBet) String() string {
	venue := b.Venue
	if venue == "" {
		venue = "?"
	}
	s := fmt.Sprintf("%s R%d #%d %s %.2fu %s", venue, b.RaceNumber, b.RunnerNo, b.RunnerName, b.Stake, b.Market)
	if b.RatedOdds > 0 {
		s += fmt.Sprintf(" (rated $%.2f)", b.RatedOdds)
	}
	return s
}

type EventMatch struct {
	Date       string
	Venue      string
	RunnerName string
	RunnerNo   int
	Score      float64 // name similarity of the match, 0..1
}
