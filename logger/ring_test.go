package logger

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// use installs l as the package logger for one test.
func use(t *testing.T, l *Logger) {
	t.Helper()
	old := std.Swap(l)
	t.Cleanup(func() { std.Store(old) })
}

func quiet(cfg Config, sinks ...Sink) *Logger {
	return &Logger{
		cfg:   cfg,
		slog:  slog.New(newTextHandler(slog.LevelError + 100)), // nothing to stderr
		ring:  newRing(cfg.Ring.size()),
		sinks: sinks,
	}
}

// The ring is the live view: newest first, wraps without losing order, and
// filters on what the API exposes.
func TestRingRecent(t *testing.T) {
	use(t, quiet(Config{Application: "test", Ring: RingConfig{Level: slog.LevelDebug, Size: 4}}))

	Info(Log{FormattedMessage: "one", UserID: "u1", ProcessID: "p1"})
	Warn(Log{FormattedMessage: "two", UserID: "u2"})
	Info(Log{FormattedMessage: "three", UserID: "u1"})
	Error(Log{FormattedMessage: "four", UserID: "u1", ProcessID: "p1"})
	Info(Log{FormattedMessage: "five", UserID: "u2"}) // evicts "one"

	all := Recent(Query{})
	if len(all) != 4 {
		t.Fatalf("ring of 4 returned %d records", len(all))
	}
	if all[0].Message != "five" || all[3].Message != "two" {
		t.Errorf("order wrong: newest %q, oldest %q", all[0].Message, all[3].Message)
	}
	if all[0].Application != "test" {
		t.Errorf("application fallback = %q", all[0].Application)
	}

	u1 := Recent(Query{UserID: "u1"})
	if len(u1) != 2 || u1[0].Message != "four" || u1[1].Message != "three" {
		t.Errorf("user filter = %+v", u1)
	}
	if got := Recent(Query{Level: "error"}); len(got) != 1 || got[0].Level != "ERROR" {
		t.Errorf("level filter = %+v", got)
	}
	if got := Recent(Query{Limit: 2}); len(got) != 2 || got[1].Message != "four" {
		t.Errorf("limit = %+v", got)
	}
	if got := Recent(Query{Since: time.Now().Add(time.Minute)}); len(got) != 0 {
		t.Errorf("since in the future returned %d", len(got))
	}
	if got := Recent(Query{ProcessID: "p1"}); len(got) != 1 || got[0].ProcessID != "p1" {
		t.Errorf("process filter = %+v", got)
	}
}

type captureSink struct {
	mu    sync.Mutex
	min   slog.Level
	got   []Record
	asked int
}

func (c *captureSink) Enabled(l slog.Level) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked++
	return l >= c.min
}
func (c *captureSink) Enqueue(r Record) { c.mu.Lock(); c.got = append(c.got, r); c.mu.Unlock() }
func (c *captureSink) Stop()            {}

// Each output takes only what meets its own level; bets skip the ring.
func TestRouting(t *testing.T) {
	sink := &captureSink{min: slog.LevelWarn}
	use(t, quiet(Config{Ring: RingConfig{Level: slog.LevelInfo}}, sink))

	Debug(Log{FormattedMessage: "debug"})
	Info(Log{FormattedMessage: "info"})
	Bet(BetLog{Message: "bet", Stake: 10})
	Warn(Log{FormattedMessage: "warn"})

	var ring []string
	for _, r := range Recent(Query{}) {
		ring = append(ring, r.Message)
	}
	if strings.Join(ring, ",") != "warn,info" {
		t.Fatalf("ring = %v, want [warn info]: no debug (below level), no bet (never)", ring)
	}
	if len(sink.got) != 1 || sink.got[0].Message != "warn" {
		t.Fatalf("sink got %+v, want only the warn", sink.got)
	}

	// A sink that wants a bet gets the bet itself on the record.
	sink.min = LevelBet
	Bet(BetLog{Message: "bet", Stake: 10})
	last := sink.got[len(sink.got)-1]
	if last.Level != "BET" || last.Bet == nil || last.Bet.Stake != 10 {
		t.Fatalf("bet record = %+v", last)
	}
}

// Request is snapshotted as JSON when logged; later changes don't reach it.
func TestRequestIsSnapshotted(t *testing.T) {
	use(t, quiet(Config{Ring: RingConfig{Level: slog.LevelDebug}}))

	req := map[string]any{"size": 10}
	Warn(Log{FormattedMessage: "odd", Request: req, Response: `{"ok":false}`})
	req["size"] = 999

	r := Recent(Query{})[0]
	if string(r.Request) != `{"size":10}` || string(r.Response) != `{"ok":false}` {
		t.Fatalf("request %s response %s", r.Request, r.Response)
	}
}

// The trace names the line that called logger.Warn, not the logger itself.
func TestTraceIsTheCallSite(t *testing.T) {
	use(t, quiet(Config{Ring: RingConfig{Level: slog.LevelDebug}}))

	Warn(Log{FormattedMessage: "where"}) // this line

	var tr struct {
		File     string `json:"file"`
		Function string `json:"function"`
	}
	if err := json.Unmarshal(Recent(Query{})[0].Trace, &tr); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(tr.File, "ring_test.go") || tr.Function != "TestTraceIsTheCallSite" {
		t.Fatalf("trace points at %s %s", tr.File, tr.Function)
	}
}
