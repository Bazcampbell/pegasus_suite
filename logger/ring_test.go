package logger

import (
	"log/slog"
	"testing"
	"time"
)

// The ring is the live view: newest first, wraps without losing order, and
// filters on what the API exposes.
func TestRingRecent(t *testing.T) {
	l := &Logger{
		slog:        slog.New(newTextHandler(slog.LevelDebug)),
		application: "test",
		ring:        newRingSink(4),
	}

	l.Info(InfoLog{Message: "one", UserID: "u1", ProcessID: "p1"})
	l.Warn(ErrorLog{Message: "two", UserID: "u2"})
	l.Info(InfoLog{Message: "three", UserID: "u1"})
	l.Error(ErrorLog{Message: "four", UserID: "u1", ProcessID: "p1"})
	l.Info(InfoLog{Message: "five", UserID: "u2"}) // evicts "one"

	all := l.Recent(Query{})
	if len(all) != 4 {
		t.Fatalf("ring of 4 returned %d records", len(all))
	}
	if all[0].Message != "five" || all[3].Message != "two" {
		t.Errorf("order wrong: newest %q, oldest %q", all[0].Message, all[3].Message)
	}

	u1 := l.Recent(Query{UserID: "u1"})
	if len(u1) != 2 || u1[0].Message != "four" || u1[1].Message != "three" {
		t.Errorf("user filter = %+v", u1)
	}

	if got := l.Recent(Query{Level: "error"}); len(got) != 1 || got[0].Level != "ERROR" {
		t.Errorf("level filter = %+v", got)
	}

	if got := l.Recent(Query{Limit: 2}); len(got) != 2 || got[1].Message != "four" {
		t.Errorf("limit = %+v", got)
	}

	if got := l.Recent(Query{Since: time.Now().Add(time.Minute)}); len(got) != 0 {
		t.Errorf("since in the future returned %d", len(got))
	}

	if got := l.Recent(Query{ProcessID: "p1"}); len(got) != 1 || got[0].ProcessID != "p1" {
		t.Errorf("process filter = %+v", got)
	}
}
