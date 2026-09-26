package report

import (
	"testing"
	"time"

	"pegasus_suite/betting"
)

func TestTotalsAdd(t *testing.T) {
	var got Totals
	for _, b := range []betting.Bet{
		{Status: betting.BetWon, Liability: 100, Profit: 250},
		{Status: betting.BetLost, Liability: 50, Profit: -50},
		{Status: betting.BetLapsed},
		{Status: betting.BetVoid},
		{Status: betting.BetPending},
	} {
		got = got.add(b)
	}
	want := Totals{Turnover: 150, Profit: 200, Attempted: 5, Accepted: 4, Pending: 1}
	if got != want {
		t.Fatalf("totals = %+v, want %+v", got, want)
	}
}

func TestUntilNextRunIsNineAMAEST(t *testing.T) {
	before := time.Date(2026, 9, 25, 8, 0, 0, 0, aest)
	if d := untilNextRun(before); d != time.Hour {
		t.Fatalf("at 8am: %v", d)
	}
	after := time.Date(2026, 9, 25, 10, 0, 0, 0, aest)
	if d := untilNextRun(after); d != 23*time.Hour {
		t.Fatalf("at 10am: %v", d)
	}
}

func TestDayIsTheAESTCalendarDay(t *testing.T) {
	// 20:00 UTC on the 24th is 06:00 AEST on the 25th
	got := day(time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC))
	if got.Format(time.DateOnly) != "2026-09-25" || got.Hour() != 0 {
		t.Fatalf("day = %v", got)
	}
}
