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
	"pegasus_suite/betting"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/engine"
)

// recorder stands in for a bookmaker session.
type recorder struct{ placed chan betting.BetRequest }

func (r *recorder) PlaceBet(req betting.BetRequest) error {
	r.placed <- req
	return nil
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

	if !p.WantsMessage(core.Update{Ref: ref}) {
		t.Fatal("process should want its own scope")
	}

	// Two seconds into the race, a full ranked field, leader is saddlecloth 5.
	p.Inbox <- core.Update{Ref: ref, Msg: tpd.Progress{
		RunningTime: 2.0,
		Order:       []string{"5", "1", "2", "3", "4"},
		Field:       []string{"1", "2", "3", "4", "5"},
	}}

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
	if n.Label != "pegasus_p1_tpdl" {
		t.Errorf("label = %q, want app_process_strategy", n.Label)
	}
	if n.TargetBot != "bot-1" || n.BookiesOverride != "3,7" {
		t.Errorf("account fields not carried: bot=%q bookies=%q", n.TargetBot, n.BookiesOverride)
	}

	// The strategy marks the race as bet, so the next tick places nothing.
	p.Inbox <- core.Update{Ref: ref, Msg: tpd.Progress{RunningTime: 3.0, Order: []string{"5", "1", "2", "3", "4"}}}
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

	if p.WantsMessage(core.Update{Ref: core.RaceRef{Scope: "US/THOROUGHBRED"}}) {
		t.Error("a US update reached an AU-only process")
	}
	if p.Running() {
		t.Error("a new process must start stopped")
	}
}
