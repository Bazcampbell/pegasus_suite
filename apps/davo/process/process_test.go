package process

import (
	"math"
	"testing"

	"pegasus_suite/apps/davo/settings"
	"pegasus_suite/engine"
)

func TestOrderStakesCashAtTheDiscountedRatedPrice(t *testing.T) {
	p := New(settings.ProcessSettings{TargetLiability: 100, MinOddsThreshold: 25}, nil, engine.TestAccount("davo", "u1", "p1", nil, nil), nil)

	o, err := p.order(Tip{Date: "2026-09-27", Venue: "EAGLE FARM", RaceNumber: 5, Runner: 3, Units: 2, Market: "PLACE", RatedOdds: 5})
	if err != nil {
		t.Fatal(err)
	}
	// rated 5.00 less 25% → min odds 4.00; $100 liability at 4.00 is $33.33 a unit
	st := o.Stake.Betmatic
	if st.MinOdds != 4 || math.Abs(st.WinStake-100.0/3) > 1e-9 || o.Unit != 2 || !st.Cash {
		t.Fatalf("stake = %+v unit %v", st, o.Unit)
	}
	if o.Side != engine.BetmaticPlace || o.BetID() != "EAGLEFARM:R5:3:20260927" {
		t.Fatalf("side %v bet id %s", o.Side, o.BetID())
	}
}

func TestOrderRefusesUnbettableOdds(t *testing.T) {
	p := New(settings.ProcessSettings{TargetLiability: 100, MinOddsThreshold: 10}, nil, engine.TestAccount("davo", "u1", "p1", nil, nil), nil)
	if _, err := p.order(Tip{RatedOdds: 1.05}); err == nil {
		t.Fatal("a tip rated 1.05 with a 10% threshold was staked")
	}
}
