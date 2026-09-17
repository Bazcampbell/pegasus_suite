package tpd

import (
	"testing"
	"time"
)

func TestRaceListPrunesByDate(t *testing.T) {
	l := NewGmaxClient("key")
	l.races["91202609111952"] = Race{RaceNumber: 1}
	l.races["91202609121952"] = Race{RaceNumber: 2}
	l.loaded["20260911"] = true
	l.loaded["20260912"] = true

	l.drop("20260911")

	if _, ok := l.races["91202609111952"]; ok {
		t.Error("dropped date left its races behind")
	}
	if _, ok := l.races["91202609121952"]; !ok {
		t.Error("dropped the wrong date")
	}
	if l.loaded["20260911"] || !l.loaded["20260912"] {
		t.Errorf("loaded set wrong: %+v", l.loaded)
	}
}

func TestRaceListLookupQueuesTheRightDate(t *testing.T) {
	l := NewGmaxClient("key")

	if _, ok := l.Lookup("91202609111952"); ok {
		t.Fatal("cold cache should miss")
	}

	select {
	case date := <-l.wanted:
		if date != "20260911" {
			t.Errorf("queued %q, want the sharecode's local date", date)
		}
	default:
		t.Fatal("miss did not queue a fetch")
	}
}

// The quiet watchdog alerts on the answer to this, so a race that is merely on
// the card must not read as due.
func TestRaceDueWithin(t *testing.T) {
	l := NewGmaxClient("key")
	now := time.Now()

	l.races["a"] = Race{Racecourse: "Later today", PostTime: now.Add(2 * time.Hour), Published: true}
	if r := l.RaceDueWithin(5 * time.Minute); r != nil {
		t.Error("a race two hours out read as due")
	}

	l.races["b"] = Race{Racecourse: "Imminent", PostTime: now.Add(2 * time.Minute), Published: true}
	race := l.RaceDueWithin(5 * time.Minute)
	if race == nil || race.Racecourse != "Imminent" {
		t.Errorf("got %q, want the imminent race", race.Racecourse)
	}

	// Just off is still due: it should be mid-feed.
	l.races["b"] = Race{Racecourse: "Just off", PostTime: now.Add(-2 * time.Minute), Published: true}
	if race := l.RaceDueWithin(5 * time.Minute); race == nil || race.Racecourse != "Just off" {
		t.Errorf("got %q, want the race that has just gone off", race.Racecourse)
	}

	l.races["b"] = Race{Racecourse: "Unpublished", PostTime: now, Published: false}
	if race := l.RaceDueWithin(5 * time.Minute); race != nil {
		t.Error("an unpublished race read as due")
	}

	l.races["b"] = Race{Racecourse: "No post time", Published: true}
	if race := l.RaceDueWithin(5 * time.Minute); race != nil {
		t.Error("a race with no post time read as due")
	}
}
