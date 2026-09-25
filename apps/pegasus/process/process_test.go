package process_test

import (
	"context"
	"testing"
	"time"

	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/apps/pegasus/dispatch"
	"pegasus_suite/apps/pegasus/process"
	"pegasus_suite/apps/pegasus/settings"
	"pegasus_suite/apps/pegasus/tpd"
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

// A process is the whole of the decision path with no kernel, no feed socket
// and no bookmaker: settings in, a race update on the inbox, a bet out.
func TestTPDLeaderBetsThroughToBetmatic(t *testing.T) {
	s := settings.ProcessSettings{
		ID:     "p1",
		UserID: "u1",
		BetmaticCredentials: engine.BetmaticCredentials{
			Username: "punter@example.com", Password: "pw", BotID: "bot-1", Bookmakers: []string{"3", "7"},
		},
		Scopes: map[string]settings.ScopeSettings{
			"US/THOROUGHBRED": {
				Stake:         engine.Stake{Betmatic: engine.BetmaticStake{WinStake: 10, MinOdds: 1.5, MaxOdds: 12}},
				BetmaticDelay: 500,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bm := &recorder{placed: make(chan betting.BetRequest, 1)}
	account := engine.TestAccount("u1", "p1", bm, nil)
	account.BotID = "bot-1"
	account.Bookmakers = []string{"3", "7"}

	p := process.New(s, dispatch.New(engine.New(ctx), account, s, nil), nil, nil, nil)

	p.Start()
	defer p.Stop()

	if !p.Running() {
		t.Fatal("process did not start")
	}

	ref := core.RaceRef{
		Provider:   core.ProviderTPD,
		Key:        "9820260917-3",
		Scope:      "US/THOROUGHBRED",
		Venue:      "98", // Belmont Park, in the TPD table
		VenueName:  "Belmont Park",
		Country:    "US",
		RaceNumber: 3,
		Code:       betmatic.THOROUGHBRED,
		Distance:   1200,
		Status:     core.StatusRunning,
	}

	if !p.Wants(ref) {
		t.Fatal("process should want its own scope")
	}

	// Two seconds into the race, a full ranked field, leader is saddlecloth 5.
	p.OfferTPD(tpd.Progress{
		RunningTime: 2.0,
		Order:       []string{"5", "1", "2", "3", "4"},
		Field:       []string{"1", "2", "3", "4", "5"},
	}, ref)

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

	if n.Selection != 5 {
		t.Errorf("selection = %d, want the leader (5)", n.Selection)
	}
	if n.EventNumber != 3 {
		t.Errorf("event = %d, want race 3", n.EventNumber)
	}
	if n.TargetProfit != 10 {
		t.Errorf("target profit = %.2f, want the scope stake x unit (10)", n.TargetProfit)
	}
	if n.Label != "pegasus_p1" {
		t.Errorf("label = %q, want pegasus_<process>", n.Label)
	}
	if n.TargetBot != "bot-1" || n.BookiesOverride != "3,7" {
		t.Errorf("account fields not carried: bot=%q bookies=%q", n.TargetBot, n.BookiesOverride)
	}

	// The strategy marks the race as bet, so the next tick places nothing.
	p.OfferTPD(tpd.Progress{RunningTime: 3.0, Order: []string{"5", "1", "2", "3", "4"}}, ref)
	select {
	case again := <-bm.placed:
		t.Fatalf("second bet placed on the same race: %+v", again)
	case <-time.After(300 * time.Millisecond):
	}
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
		ID:                  "p1",
		UserID:              "u1",
		BetmaticCredentials: engine.BetmaticCredentials{Username: "punter@example.com", Password: "pw"},
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
	p := process.New(s, dispatch.New(engine.New(ctx), engine.TestAccount("u1", "p1", bm, nil), s, nil), getRace, nil, nil)
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
}
