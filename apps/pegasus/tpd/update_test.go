package tpd

import (
	"testing"
	"time"

	"pegasus_suite/apps/pegasus/core"

	"pegasus_suite/betting/betmatic"
)

// stubList answers the way the race list would for the two courses the
// fixtures use.
func stubList(sharecode string) (Race, bool) {
	course, _, err := ParseSharecode(sharecode)
	if err != nil {
		return Race{}, false
	}

	names := map[string]string{"91": "Mohawk Park", "30": "Lingfield Park"}
	name, ok := names[course]
	if !ok {
		return Race{}, false
	}

	return Race{Racecourse: name, Country: "CA", RaceNumber: 4, Published: true}, true
}

// A real pre-off packet off the wire, Mohawk Park.
const preOff = `{"K":5,"T":"2026-09-11T23:41:52.6Z","I":"91202609111952","G":"","L":1609.3,"S":0.00,"C":0.00,"R":0.00,"V":0.0,"P":1609.3,"O":[],"F":["8","2","7","1","5","6","4"],"B":[],"W":0}`

// The worked example from GX-UG-00020, Lingfield Park, mid-race.
const midRace = `{"K":5,"T":"2016-01-12T13:11:10.9Z","I":"30201601121310","G":"1f","L":100.6,"S":10.61,"C":40.13,"R":46.72,"V":14.8,"P":87.5,"O":["3","5","1","2","6"],"F":["2","1","3","5","6"],"B":[0,0.4,0.4,0.9,1.5],"W":0}`

// harness wires a datagram to updates exactly the way the supervisor does, so
// the tests exercise the real composition rather than a stand-in for it.
type harness struct {
	tracker *Tracker
	got     []core.Update
}

func newHarness() *harness {
	return &harness{tracker: NewTracker(stubList)}
}

func (h *harness) feed(t *testing.T, datagram string) {
	t.Helper()

	bad := decodeDatagram([]byte(datagram), func(p Progress) {
		if ref, ok := h.tracker.Ref(p); ok {
			h.got = append(h.got, core.Update{Ref: ref, Msg: p})
		}
	})
	if bad != 0 {
		t.Fatalf("decode: %d bad payloads", bad)
	}
}

func progress(t *testing.T, u core.Update) Progress {
	t.Helper()
	p, ok := u.Msg.(Progress)
	if !ok {
		t.Fatalf("update carried %T, want tpd.Progress", u.Msg)
	}
	return p
}

func TestPreOffPacket(t *testing.T) {
	h := newHarness()
	h.feed(t, preOff)

	if len(h.got) != 1 {
		t.Fatalf("got %d updates, want 1", len(h.got))
	}

	ref := h.got[0].Ref
	if ref.Key != "91202609111952" || ref.Venue != "91" || ref.VenueName != "Mohawk Park" {
		t.Errorf("identity: key=%q venue=%q name=%q", ref.Key, ref.Venue, ref.VenueName)
	}
	if ref.Scope != "CA/HARNESS" || ref.Code != betmatic.HARNESS || ref.Country != "CA" {
		t.Errorf("scope=%q code=%v country=%q", ref.Scope, ref.Code, ref.Country)
	}
	if ref.Status != core.StatusPreOff {
		t.Errorf("status %v, want pre-off", ref.Status)
	}
	if ref.Distance != 1609.3 {
		t.Errorf("distance %v, want the pre-off progress", ref.Distance)
	}

	// The packet itself passes through untouched for the strategy to read.
	p := progress(t, h.got[0])
	if len(p.Field) != 7 || p.Field[0] != "8" || len(p.Order) != 0 || p.Progress != 1609.3 {
		t.Errorf("packet altered in transit: %+v", p)
	}
}

func TestMidRacePacket(t *testing.T) {
	h := newHarness()
	h.feed(t, midRace)

	u := h.got[0]
	if u.Ref.VenueName != "Lingfield Park" || u.Ref.Status != core.StatusRunning {
		t.Errorf("venue %q status %v", u.Ref.VenueName, u.Ref.Status)
	}

	p := progress(t, u)
	if p.RunningTime != 46.72 || p.LeaderSpeed != 14.8 || p.Progress != 87.5 {
		t.Errorf("R/V/P wrong: %v %v %v", p.RunningTime, p.LeaderSpeed, p.Progress)
	}
	if len(p.Order) != 5 || p.Order[0] != "3" || p.Order[4] != "6" {
		t.Errorf("order wrong: %v", p.Order)
	}
	if len(p.DistanceBack) != 5 || p.DistanceBack[0] != 0 || p.DistanceBack[4] != 1.5 {
		t.Errorf("margins wrong: %v", p.DistanceBack)
	}
}

// Gmax's start signal is a third-party feed that fails at some tracks, leaving
// running time at zero while everything else stays valid.
func TestStartDetectionFailureStillReadsAsRunning(t *testing.T) {
	h := newHarness()

	h.feed(t, preOff)
	h.feed(t, `{"K":5,"T":"2026-09-11T23:43:00.0Z","I":"91202609111952","G":"","L":1609.3,"S":0,"C":0,"R":0.00,"V":11.2,"P":1400.0,"O":["8","2"],"F":["8","2"],"B":[0,1.2],"W":0}`)

	ref := h.got[1].Ref
	if ref.Status != core.StatusRunning {
		t.Errorf("status %v, want running from falling progress", ref.Status)
	}
	if ref.Distance != 1609.3 {
		t.Errorf("official distance %v, want the pre-off maximum", ref.Distance)
	}
}

func TestFinishedWhenProgressReachesZero(t *testing.T) {
	h := newHarness()

	h.feed(t, preOff)
	h.feed(t, `{"K":5,"T":"2026-09-11T23:44:00.0Z","I":"91202609111952","G":"0f","L":0,"S":12.1,"C":118.4,"R":118.40,"V":0.0,"P":0,"O":["8","2"],"F":["8","2"],"B":[0,1.2],"W":0}`)

	if ref := h.got[1].Ref; ref.Status != core.StatusFinished {
		t.Errorf("status %v, want finished", ref.Status)
	}
}

func TestDuplicateAndReorderedPacketsDropped(t *testing.T) {
	h := newHarness()

	h.feed(t, preOff)
	h.feed(t, `{"K":5,"T":"2026-09-11T23:41:53.1Z","I":"91202609111952","G":"","L":1609.3,"S":0,"C":0,"R":0,"V":0,"P":1609.3,"O":[],"F":["8","2"],"B":[],"W":0}`)
	h.feed(t, preOff) // the duplicate a second parallel feed delivers
	h.feed(t, `{"K":5,"T":"2026-09-11T23:41:52.9Z","I":"91202609111952","G":"","L":1609.3,"S":0,"C":0,"R":0,"V":0,"P":1609.3,"O":[],"F":["8"],"B":[],"W":0}`)

	if len(h.got) != 2 {
		t.Fatalf("got %d updates, want 2", len(h.got))
	}
}

func TestNonProgressPacketsIgnored(t *testing.T) {
	h := newHarness()

	h.feed(t, `{"K":2,"T":"2026-09-11T23:41:52.6Z","I":"91202609111952"}`)
	if len(h.got) != 0 {
		t.Fatalf("published %d updates for packet type 2", len(h.got))
	}

	if bad := decodeDatagram([]byte("not json at all"), func(Progress) {}); bad != 1 {
		t.Errorf("got %d bad payloads, want 1", bad)
	}
}

// Gmax reserve bits 5 to 7 and tell subscribers to tolerate extension. A warning
// flag the spec does not describe yet must not cost us the packet.
func TestWarningBitsSurviveDecoding(t *testing.T) {
	h := newHarness()
	h.feed(t, `{"K":5,"T":"2026-09-11T23:41:52.6Z","I":"91202609111952","G":"","L":1609.3,"S":0,"C":0,"R":0,"V":0,"P":1609.3,"O":[],"F":["8","2"],"B":[],"W":4104}`)

	if len(h.got) != 1 {
		t.Fatalf("got %d updates, want 1", len(h.got))
	}

	p := progress(t, h.got[0])
	if p.Warnings&WarnAssignment == 0 {
		t.Errorf("known bit lost among unknown ones: %v", p.Warnings)
	}
	if p.Warnings&WarnStart != 0 {
		t.Errorf("start warning set when it should not be: %v", p.Warnings)
	}
}

func TestJSONArrayPayload(t *testing.T) {
	h := newHarness()
	h.feed(t, "["+preOff+","+midRace+"]")

	if len(h.got) != 2 {
		t.Fatalf("got %d updates, want 2", len(h.got))
	}
}

func TestUnknownCourseSkipped(t *testing.T) {
	h := newHarness()
	h.feed(t, `{"K":5,"T":"2026-09-11T23:41:52.6Z","I":"99202609111952","G":"","L":100,"S":0,"C":0,"R":0,"V":0,"P":100,"O":[],"F":["1"],"B":[],"W":0}`)

	if len(h.got) != 0 {
		t.Fatalf("published %d updates for an unmapped course", len(h.got))
	}
}

// An unlisted race resolves false and must not be cached as tracked, or the
// race list warming up a second later would never be picked up.
func TestUnlistedRaceRetriesOnceListed(t *testing.T) {
	listed := false
	tr := NewTracker(func(sharecode string) (Race, bool) {
		if !listed {
			return Race{}, false
		}
		return Race{Racecourse: "Mohawk Park", Country: "CA", RaceNumber: 4}, true
	})

	var p Progress
	decodeDatagram([]byte(preOff), func(got Progress) { p = got })

	if _, ok := tr.Ref(p); ok {
		t.Fatal("resolved against a cold race list")
	}

	listed = true
	if _, ok := tr.Ref(p); !ok {
		t.Error("did not retry once the race list had the race")
	}
}

func TestSharecode(t *testing.T) {
	course, off, err := ParseSharecode("91202609111952")
	if err != nil || course != "91" || !off.Equal(time.Date(2026, 9, 11, 19, 52, 0, 0, time.UTC)) {
		t.Fatalf("sharecode: %q %v %v", course, off, err)
	}

	if _, _, err := ParseSharecode("91"); err == nil {
		t.Error("short sharecode should error")
	}
}
