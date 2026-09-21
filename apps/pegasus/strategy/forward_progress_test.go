package strategy

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"pegasus_suite/apps/pegasus/core"
	triples "pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/betting/betmatic"
)

// oldExtremes is the calculation as it was before the rewrite, kept verbatim
// so the new one can be held to it.
func oldExtremes(start, current triples.RaceMessage) (best, worst int, ok bool) {
	startMap := make(map[int]triples.LiveDataSet, len(start.LiveDataSets))
	for _, h := range start.LiveDataSets {
		startMap[int(h.SaddleclothNumber)] = h
	}
	displacements := make(map[int]core.Vector, len(current.LiveDataSets))
	var fieldVector core.Vector
	var count float64
	for _, curr := range current.LiveDataSets {
		startHorse, seen := startMap[int(curr.SaddleclothNumber)]
		if !seen {
			continue
		}
		d := curr.GPSCoordinates.Sub(startHorse.GPSCoordinates)
		displacements[int(curr.SaddleclothNumber)] = d
		fieldVector = fieldVector.Add(d)
		count++
	}
	if count == 0 {
		return 0, 0, false
	}
	forward := fieldVector.Div(count).Normalise()
	bestProgress, worstProgress := math.Inf(-1), math.Inf(1)
	for saddlecloth, d := range displacements {
		progress := d.Dot(forward)
		if progress > bestProgress {
			bestProgress, best = progress, saddlecloth
		}
		if progress < worstProgress {
			worstProgress, worst = progress, saddlecloth
		}
	}
	if best < 1 && worst < 1 {
		return 0, 0, false
	}
	return best, worst, true
}

func field(rng *rand.Rand, runners int, at time.Time, from *triples.RaceMessage) triples.RaceMessage {
	m := triples.RaceMessage{Timestamp: at.Format("2006-01-02T15:04:05.000Z")}
	for i := 0; i < runners; i++ {
		pos := core.Vector{Latitude: -27 + rng.Float64()*1e-3, Longitude: 153 + rng.Float64()*1e-3}
		if from != nil {
			pos = from.LiveDataSets[i].GPSCoordinates.Add(core.Vector{Latitude: rng.Float64() * 1e-4, Longitude: rng.Float64() * 5e-4})
		}
		m.LiveDataSets = append(m.LiveDataSets, triples.LiveDataSet{SaddleclothNumber: core.FlexInt(i + 1), GPSCoordinates: pos})
	}
	return m
}

func startPositions(m triples.RaceMessage) map[int]core.Vector {
	out := make(map[int]core.Vector, len(m.LiveDataSets))
	for _, h := range m.LiveDataSets {
		out[int(h.SaddleclothNumber)] = h.GPSCoordinates
	}
	return out
}

func TestExtremesMatchTheOldCalculation(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	s := NewForwardProgress()
	t0 := time.Date(2026, 9, 19, 5, 0, 0, 0, time.UTC)

	for i := 0; i < 5000; i++ {
		runners := 2 + rng.Intn(18)
		start := field(rng, runners, t0, nil)
		current := field(rng, runners, t0.Add(time.Second), &start)
		if i%7 == 0 { // a runner missing from the start (joined late) is skipped by both
			start.LiveDataSets = start.LiveDataSets[1:]
		}

		wantBest, wantWorst, wantOK := oldExtremes(start, current)
		best, worst, ok := s.extremes(startPositions(start), current.LiveDataSets)
		if best != wantBest || worst != wantWorst || ok != wantOK {
			t.Fatalf("field %d: got (%d, %d, %v), old gave (%d, %d, %v)", i, best, worst, ok, wantBest, wantWorst, wantOK)
		}
	}
}

type fpRun struct {
	t   *testing.T
	s   *ForwardProgress
	ref core.RaceRef
	get func(core.RaceRef) *core.BetfairRace
}

func newFPRun(t *testing.T, code betmatic.RacingCode, distance int) *fpRun {
	return &fpRun{
		t:   t,
		s:   NewForwardProgress(),
		ref: core.RaceRef{Provider: core.ProviderTripleS, Key: "k", Scope: "AU/X", Code: code, Status: core.StatusRunning},
		get: func(core.RaceRef) *core.BetfairRace { return &core.BetfairRace{Distance: distance} },
	}
}

func (r *fpRun) tick(m triples.RaceMessage, bfDelay, bmDelay int64) core.Decision {
	r.t.Helper()
	d, err := r.s.Select(m, r.ref, bfDelay, bmDelay, r.get)
	if err != nil {
		r.t.Fatal(err)
	}
	return d
}

func TestForwardProgressDecisions(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	t0 := time.Date(2026, 9, 19, 5, 0, 0, 0, time.UTC)
	start := field(rng, 8, t0, nil)
	early := field(rng, 8, t0.Add(400*time.Millisecond), &start)
	late := field(rng, 8, t0.Add(time.Second), &start)
	best, worst, _ := oldExtremes(start, late)

	t.Run("waits for each provider's delay, bets once", func(t *testing.T) {
		r := newFPRun(t, betmatic.THOROUGHBRED, 1000)
		if d := r.tick(start, 500, 1000); !d.Tracking || len(d.Bets) != 0 {
			t.Fatalf("first running message should only start the race: %+v", d)
		}
		if d := r.tick(early, 500, 1000); len(d.Bets) != 0 {
			t.Fatalf("bet before any delay: %+v", d)
		}
		d := r.tick(late, 500, 1000)
		if len(d.Bets) != 3 { // betmatic win, betfair back, betfair lay
			t.Fatalf("bets = %+v", d.Bets)
		}
		if d.Bets[0].Runner != best || d.Bets[0].Unit != 1 || d.Bets[2].Runner != worst {
			t.Fatalf("best %d worst %d, bets %+v", best, worst, d.Bets)
		}
		if d := r.tick(late, 500, 1000); len(d.Bets) != 0 {
			t.Fatalf("bet twice: %+v", d)
		}
	})

	t.Run("unit by code and distance", func(t *testing.T) {
		for _, c := range []struct {
			code     betmatic.RacingCode
			distance int
			unit     float64
		}{
			{betmatic.THOROUGHBRED, 1000, 1}, {betmatic.THOROUGHBRED, 1200, 0.8}, {betmatic.THOROUGHBRED, 1400, 0.6},
			{betmatic.HARNESS, 1800, 1}, {betmatic.HARNESS, 2600, 0.8},
		} {
			r := newFPRun(t, c.code, c.distance)
			r.tick(start, 0, 0)
			if d := r.tick(late, 0, 0); len(d.Bets) == 0 || d.Bets[0].Unit != c.unit {
				t.Fatalf("%s %dm: %+v, want unit %v", c.code, c.distance, d.Bets, c.unit)
			}
		}
	})

	t.Run("long thoroughbreds and unknown codes are left alone", func(t *testing.T) {
		for _, r := range []*fpRun{newFPRun(t, betmatic.THOROUGHBRED, 1600), newFPRun(t, betmatic.GREYHOUNDS, 500)} {
			if d := r.tick(start, 0, 0); d.Tracking || len(d.Bets) != 0 {
				t.Fatalf("%s: %+v", r.ref.Code, d)
			}
			if d := r.tick(late, 0, 0); d.Tracking || len(d.Bets) != 0 {
				t.Fatalf("%s bet: %+v", r.ref.Code, d)
			}
		}
	})

	t.Run("finished forgets the race", func(t *testing.T) {
		r := newFPRun(t, betmatic.THOROUGHBRED, 1000)
		r.tick(start, 0, 0)
		r.ref.Status = core.StatusFinished
		r.tick(late, 0, 0)
		if len(r.s.races) != 0 {
			t.Fatalf("race kept after finish: %v", r.s.races)
		}
	})
}

func BenchmarkForwardProgressTick(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	t0 := time.Date(2026, 9, 19, 5, 0, 0, 0, time.UTC)
	start := field(rng, 12, t0, nil)
	msg := field(rng, 12, t0.Add(time.Second), &start)
	s := NewForwardProgress()
	ref := core.RaceRef{Provider: core.ProviderTripleS, Key: "k", Code: betmatic.THOROUGHBRED, Status: core.StatusRunning}
	get := func(core.RaceRef) *core.BetfairRace { return &core.BetfairRace{Distance: 1000} }
	s.Select(start, ref, 1e12, 1e12, get)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Select(msg, ref, 1e12, 1e12, get)
	}
}

func BenchmarkForwardProgressExtremes(b *testing.B) {
	rng := rand.New(rand.NewSource(4))
	t0 := time.Date(2026, 9, 19, 5, 0, 0, 0, time.UTC)
	start := field(rng, 12, t0, nil)
	cur := field(rng, 12, t0.Add(time.Second), &start)
	positions := startPositions(start)
	s := NewForwardProgress()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.extremes(positions, cur.LiveDataSets)
	}
}
