package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betmatic"
)

type recorder struct {
	placed chan betting.BetRequest
	err    error
}

func (r *recorder) PlaceBet(req betting.BetRequest) (string, error) {
	r.placed <- req
	return "id", r.err
}

func newRecorder() *recorder { return &recorder{placed: make(chan betting.BetRequest, 10)} }

func order(a *Account, runner int) Order {
	return Order{
		Account: a,
		Race:    Race{Date: "2026-09-25", Venue: "EAGLE FARM", Code: betmatic.THOROUGHBRED, Number: 3, MarketID: "1.23"},
		Runner:  runner,
		Side:    BetmaticWin,
		Unit:    1,
		Stake:   Stake{Betmatic: BetmaticStake{WinStake: 10}},
	}
}

// accounts builds accounts for one user sharing one set of claims.
func accounts(keys ...[2]string) []*Account {
	c := newClaims()
	out := make([]*Account, len(keys))
	for i, k := range keys {
		out[i] = &Account{App: k[0], UserID: "u1", ProcessID: k[1], betmatic: newRecorder(), claims: c}
	}
	return out
}

func placed(a *Account) int { return len(a.betmatic.(*recorder).placed) }

func TestBetID(t *testing.T) {
	if got := order(nil, 7).BetID(); got != "EAGLEFARM:R3:7:20260925" {
		t.Fatalf("bet id = %q", got)
	}
}

func TestDedupeIsPerUserAndApp(t *testing.T) {
	eng := New(context.Background())
	acc := accounts([2]string{"pegasus", "a"}, [2]string{"pegasus", "b"}, [2]string{"davo", "c"}, [2]string{"davo", "d"})
	pegA, pegB, davoC, davoD := acc[0], acc[1], acc[2], acc[3]

	eng.Place(order(pegA, 4))
	eng.Place(order(pegA, 4))  // same process again: blocked
	eng.Place(order(pegB, 4))  // same app, other process: allowed
	eng.Place(order(davoC, 4)) // other app: blocked
	eng.Place(order(davoC, 5)) // other runner: allowed
	eng.Place(order(davoD, 5)) // same app, other process: allowed

	for _, c := range []struct {
		name string
		a    *Account
		want int
	}{{"pegasus a", pegA, 1}, {"pegasus b", pegB, 1}, {"davo c", davoC, 1}, {"davo d", davoD, 1}} {
		if got := placed(c.a); got != c.want {
			t.Errorf("%s placed %d, want %d", c.name, got, c.want)
		}
	}
}

func TestOneProcessMayBetEachProviderOnce(t *testing.T) {
	c := newClaims()
	a := &Account{App: "pegasus", ProcessID: "a"}
	if !c.take(a, "x", "", betting.ProviderBetmatic) || !c.take(a, "x", "", betting.ProviderBetfair) {
		t.Fatal("betmatic then betfair on one runner should both be allowed")
	}
	if c.take(a, "x", "", betting.ProviderBetfair) {
		t.Fatal("a second betfair bet on the runner was allowed")
	}
}

func TestFailedBetFreesTheRunner(t *testing.T) {
	eng := New(context.Background())
	acc := accounts([2]string{"pegasus", "a"}, [2]string{"davo", "c"})
	acc[0].betmatic.(*recorder).err = errors.New("rejected")

	eng.Place(order(acc[0], 4))
	eng.Place(order(acc[1], 4))
	if placed(acc[1]) != 1 {
		t.Fatal("davo was blocked by a pegasus bet that failed")
	}
}

func TestClaimsDropOnMarketCloseAndExpiry(t *testing.T) {
	c := newClaims()
	a := &Account{App: "pegasus", ProcessID: "a"}
	c.take(a, "streamed", "1.23", betting.ProviderBetfair)
	c.take(a, "tip", "", betting.ProviderBetmatic)

	c.drop("1.23", time.Time{})
	if _, ok := c.m["streamed"]; ok {
		t.Fatal("claim kept after its market closed")
	}
	if _, ok := c.m["tip"]; !ok {
		t.Fatal("claim with no market dropped on another market's close")
	}

	c.drop("", time.Now().Add(time.Second))
	if len(c.m) != 0 {
		t.Fatalf("claims kept past expiry: %v", c.m)
	}
}
