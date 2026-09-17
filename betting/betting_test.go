// betting/betting_test.go

package betting_test

import (
	"errors"
	"testing"

	"racing_wagering/betting"
	"racing_wagering/betting/betfair"
	"racing_wagering/betting/betmatic"
)

// The interface exists so ENGINE can pool both providers behind one type. The
// per-package `var _ betting.Client` assertions catch a client drifting; this
// catches the pool's own constraint drifting, from the outside.
func TestBothClientsArePoolable(t *testing.T) {
	var clients []betting.Client = []betting.Client{
		(*betmatic.Client)(nil),
		(*betfair.Client)(nil),
	}
	if len(clients) != 2 {
		t.Fatal("unreachable")
	}
}

func TestProviderRoundTrip(t *testing.T) {
	cases := map[string]betting.Provider{
		"betmatic":   betting.ProviderBetmatic,
		"BETMATIC":   betting.ProviderBetmatic,
		"  Betfair ": betting.ProviderBetfair,
	}
	for in, want := range cases {
		got, err := betting.ParseProvider(in)
		if err != nil {
			t.Errorf("ParseProvider(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseProvider(%q) = %q, want %q", in, got, want)
		}
	}

	if _, err := betting.ParseProvider("tote"); !errors.Is(err, betting.ErrUnknownProvider) {
		t.Errorf("expected ErrUnknownProvider, got %v", err)
	}
}

// A request reaching the wrong client is the one failure the type system cannot
// catch — the pool hands back a betting.Client, not a concrete type.
func TestPlaceBetRejectsForeignRequest(t *testing.T) {
	var betmaticClient betting.Client = &betmatic.Client{}
	err := betmaticClient.PlaceBet(betfair.BetRequest{
		MarketID: "1.23", SelectionID: 47, Side: betfair.SideBack, Price: 3.4, Size: 10,
	})
	if !errors.Is(err, betting.ErrWrongProvider) {
		t.Errorf("betmatic client took a betfair request: %v", err)
	}

	var betfairClient betting.Client = &betfair.Client{}
	err = betfairClient.PlaceBet(betmatic.NotificationRequest{
		Type: betmatic.FIXED_PROFIT, Sports: "RACING",
	})
	if !errors.Is(err, betting.ErrWrongProvider) {
		t.Errorf("betfair client took a betmatic request: %v", err)
	}
}

// Requests must report the provider they belong to, since that is what routes
// them and what ErrWrongProvider is checked against.
func TestRequestsReportTheirProvider(t *testing.T) {
	if got := (betmatic.NotificationRequest{}).Provider(); got != betting.ProviderBetmatic {
		t.Errorf("betmatic request reports %q", got)
	}
	if got := (betfair.BetRequest{}).Provider(); got != betting.ProviderBetfair {
		t.Errorf("betfair request reports %q", got)
	}
}
