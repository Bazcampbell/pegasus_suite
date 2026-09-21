package betmatic

import (
	"net/http"
	"testing"

	"pegasus_suite/betting"
	"pegasus_suite/platform/util"
)

func TestGetBetResultsANotification(t *testing.T) {
	var path, auth string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Write([]byte(`{"bets": [
			{"id": 1, "amount": "6",  "current_odds": "4.6", "profit": "21.60", "status": "WON"},
			{"id": 2, "amount": 4,    "current_odds": 4.6,   "profit": 14.4,    "status": "won"},
			{"id": 3, "amount": 0,    "current_odds": 0,     "profit": 0,       "status": "FAILED", "submit_error": "limit"}
		]}`))
	})

	b, err := c.GetBet("5001")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/bet/notification/5001/" || auth != "Token test-token" {
		t.Fatalf("called %s with %q", path, auth)
	}
	want := betting.Bet{ID: "5001", Provider: betting.ProviderBetmatic, Status: betting.BetWon, Stake: 10, Odds: 4.6, Profit: 36}
	if b.ID != want.ID || b.Provider != want.Provider || b.Status != want.Status ||
		b.Stake != want.Stake || !near(b.Odds, want.Odds) || !near(b.Profit, want.Profit) {
		t.Fatalf("got %+v, want %+v", b, want)
	}
}

func TestGetBetPropagatesFailure(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := c.GetBet("5001"); err == nil {
		t.Fatal("a 401 read as a bet; the resulter would treat it as pending forever")
	}
}

func TestFoldNotificationBets(t *testing.T) {
	bet := func(amount, odds, profit float64, status string) NotificationBet {
		return NotificationBet{Amount: util.FlexFloat(amount), CurrentOdds: util.FlexFloat(odds), Profit: util.FlexFloat(profit), Status: status}
	}
	cases := []struct {
		name string
		bets []NotificationBet
		want betting.BetStatus
	}{
		{"nothing yet", nil, betting.BetPending},
		{"one bookie still open", []NotificationBet{bet(5, 3, 10, "WON"), bet(5, 3, 0, "PENDING")}, betting.BetPending},
		{"unknown status stays pending", []NotificationBet{bet(5, 3, 10, "SOMETHING_NEW")}, betting.BetPending},
		{"lost", []NotificationBet{bet(10, 3, -10, "LOST")}, betting.BetLost},
		{"void", []NotificationBet{bet(10, 3, 0, "VOID")}, betting.BetVoid},
		{"every bookie refused", []NotificationBet{bet(0, 0, 0, "FAILED"), bet(0, 0, 0, "REJECTED")}, betting.BetLapsed},
	}
	for _, c := range cases {
		if got := foldNotificationBets(c.bets).Status; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func near(a, b float64) bool { d := a - b; return d < 1e-9 && d > -1e-9 }
