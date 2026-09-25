package process_test

import (
	"context"
	"testing"
	"time"

	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/dispatch"
	"pegasus_suite/apps/pegasus/process"
	"pegasus_suite/apps/pegasus/settings"
	triples "pegasus_suite/apps/pegasus/triples"
	"pegasus_suite/betting"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/engine"
)

// recorder stands in for a bookmaker session.
type recorder struct{ placed chan betting.BetRequest }

func (r *recorder) PlaceBet(req betting.BetRequest) (string, error) {
	r.placed <- req
	return "", nil
}

func TestProcessIgnoresOtherScopes(t *testing.T) {
	s := settings.ProcessSettings{
		ID: "p1", UserID: "u1",
		BetmaticCredentials: engine.BetmaticCredentials{Username: "a@b.c", Password: "pw"},
		Scopes: map[string]settings.ScopeSettings{"AU/THOROUGHBRED": {
			Stake: engine.Stake{Betmatic: engine.BetmaticStake{WinStake: 5}}, BetmaticDelay: 500,
		}},
	}
	p := process.New(s, dispatch.New(nil, engine.TestAccount("u1", "p1", nil, nil), s, nil), nil, nil, nil)

	if p.Running() {
		t.Error("a new process must start stopped")
	}
	if p.Wants(core.RaceRef{Scope: "AU/THOROUGHBRED"}) {
		t.Error("a stopped process took a message")
	}

	p.Start()
	defer p.Stop()

	if p.Wants(core.RaceRef{Scope: "US/THOROUGHBRED"}) {
		t.Error("a US update reached an AU-only process")
	}
	if !p.Wants(core.RaceRef{Scope: "AU/THOROUGHBRED"}) {
		t.Error("a running process refused its own scope")
	}
}

// Raw Triple-S messages in, a Betmatic bet on the forward-progress leader out.
func TestForwardProgressBetsThroughToBetmatic(t *testing.T) {
	s := settings.ProcessSettings{
		ID:     "p1",
		UserID: "u1",
		BetmaticCredentials: engine.BetmaticCredentials{
			Username: "punter@example.com", Password: "pw", BotID: "bot-1", Bookmakers: []string{"3", "7"},
		},
		Scopes: map[string]settings.ScopeSettings{
			"AU/THOROUGHBRED": {
				Stake:         engine.Stake{Betmatic: engine.BetmaticStake{WinStake: 10, MinOdds: 1.5, MaxOdds: 12}},
				BetmaticDelay: 500,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bm := &recorder{placed: make(chan betting.BetRequest, 1)}
	getRace := func(core.RaceRef) *core.BetfairRace { return &core.BetfairRace{Distance: 1000} }
	account := engine.TestAccount("u1", "p1", bm, nil)
	account.BotID = "bot-1"
	account.Bookmakers = []string{"3", "7"}
	p := process.New(s, dispatch.New(engine.New(ctx), account, s, nil), getRace, nil, nil)
	p.Start()
	defer p.Stop()

	start := time.Date(2026, 9, 19, 5, 0, 0, 0, time.UTC)
	msg := func(at time.Time, moved map[int]float64) triples.RaceMessage {
		m := triples.RaceMessage{
			CountryRaceCode: "AUS/RQLD",
			Timestamp:       at.Format("2006-01-02T15:04:05.000Z"),
			Venue:           triples.Venue{Name: "Townsville"},
			EventDate:       "2026-09-19",
			RaceNumber:      4,
			RaceState:       triples.RaceRunning,
		}
		for cloth := 1; cloth <= 6; cloth++ {
			m.LiveDataSets = append(m.LiveDataSets, triples.LiveDataSet{
				SaddleclothNumber: core.FlexInt(cloth),
				GPSCoordinates:    core.Vector{Latitude: -19.3 + float64(cloth)*1e-5, Longitude: 146.8 + moved[cloth]},
			})
		}
		return m
	}

	offer := func(m triples.RaceMessage) {
		t.Helper()
		ref, ok := m.Ref()
		if !ok {
			t.Fatal("message has no race identity")
		}
		p.OfferTripleS(m, ref)
	}

	offer(msg(start, nil))
	offer(msg(start.Add(600*time.Millisecond), map[int]float64{1: 1e-4, 2: 2e-4, 3: 5e-4, 4: 3e-4, 5: 1e-4, 6: 2e-4}))

	var req betting.BetRequest
	select {
	case req = <-bm.placed:
	case <-time.After(2 * time.Second):
		t.Fatal("no bet reached the betmatic session")
	}

	n, ok := req.(betmatic.NotificationRequest)
	if !ok {
		t.Fatalf("got %T, want a betmatic notification", req)
	}
	if n.Selection != 3 {
		t.Errorf("selection = %d, want the runner that moved furthest (3)", n.Selection)
	}
	if n.EventNumber != 4 || n.Label != "pegasus_p1" {
		t.Errorf("event %d label %q", n.EventNumber, n.Label)
	}
	if n.TargetBot != "bot-1" || n.BookiesOverride != "3,7" {
		t.Errorf("account fields not carried: bot=%q bookies=%q", n.TargetBot, n.BookiesOverride)
	}
}
