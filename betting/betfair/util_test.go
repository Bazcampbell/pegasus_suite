// betfair/util_test.go

package betfair

import "testing"

func TestRacingCode(t *testing.T) {
	races := func(names ...string) map[int]*Race {
		out := make(map[int]*Race, len(names))
		for i, name := range names {
			out[i+1] = &Race{Number: i + 1, Name: name}
		}
		return out
	}

	cases := []struct {
		name  string
		races map[int]*Race
		want  RacingCode
	}{
		{"harness pace", races("R1 2030m Pace", "R2 1609m Pace"), TROT},
		{"harness trot", races("R1 2100m Trot"), TROT},
		{"marker at end of name", races("R1 2030m Pace"), TROT},
		{"marker mid name", races("R4 1609m Trot Final"), TROT},
		{"marker on a later race only", races("R1 1609m Mobile", "R2 2030m Pace"), TROT},
		{"thoroughbred", races("R1 1200m Mdn", "R2 1600m Hcp"), THOROUGHBRED},
		{"no marker mid-word", races("R1 1200m Spacegoat Stks"), THOROUGHBRED},
		{"no marker as prefix", races("R1 1200m Pacemaker Hcp"), THOROUGHBRED},
		{"no markets", races(), THOROUGHBRED},
	}

	for _, c := range cases {
		if got := racingCode(c.races); got != c.want {
			t.Errorf("%s: racingCode() = %v, want %v", c.name, got, c.want)
		}
	}
}
