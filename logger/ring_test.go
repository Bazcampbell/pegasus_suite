package logger

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// use installs a logger built from cfg for one test and returns a func that
// drains it, so everything logged so far has been written.
func use(t *testing.T, cfg Config, sinks ...Sink) (drain func()) {
	t.Helper()
	cfg.StdErrLevel = slog.LevelError + 100
	l := start(cfg, sinks...)
	old := std.Swap(l)
	t.Cleanup(func() { std.Store(old) })
	return l.stop
}

func TestRingRecent(t *testing.T) {
	drain := use(t, Config{Application: "test", Ring: RingConfig{Level: slog.LevelDebug, Size: 4}})

	Info(Log{Message: "one", UserID: "u1", ProcessID: "p1"})
	Warn(Log{Message: "two", UserID: "u2"})
	Info(Log{Message: "three", UserID: "u1"})
	Error(Log{Message: "four", UserID: "u1", ProcessID: "p1"})
	Info(Log{Message: "five", UserID: "u2"}) // evicts "one"
	drain()

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
	mu  sync.Mutex
	min slog.Level
	got []Record
}

func (c *captureSink) Enabled(l slog.Level) bool { return l >= c.min }
func (c *captureSink) Enqueue(r Record)          { c.mu.Lock(); c.got = append(c.got, r); c.mu.Unlock() }
func (c *captureSink) Stop()                     {}

func TestRouting(t *testing.T) {
	sink := &captureSink{min: slog.LevelWarn}
	drain := use(t, Config{Ring: RingConfig{Level: slog.LevelInfo, Size: 10}}, sink)

	Debug(Log{Message: "debug"})
	Info(Log{Message: "info"})
	Bet(BetLog{Message: "bet", Stake: 10})
	Warn(Log{Message: "warn"})
	drain()

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
}

func TestBetReachesASinkThatWantsIt(t *testing.T) {
	sink := &captureSink{min: LevelBet}
	drain := use(t, Config{}, sink)

	Bet(BetLog{Message: "bet", Stake: 10})
	drain()

	if len(sink.got) != 1 || sink.got[0].Level != "BET" || sink.got[0].Bet == nil || sink.got[0].Bet.Stake != 10 {
		t.Fatalf("bet record = %+v", sink.got)
	}
}

func TestFullQueueDropsInsteadOfBlocking(t *testing.T) {
	l := &Logger{min: slog.LevelDebug, queue: make(chan entry, 1)}
	l.emit(slog.LevelInfo, Log{Message: "kept"})

	done := make(chan struct{})
	go func() { l.emit(slog.LevelInfo, Log{Message: "dropped"}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit blocked on a full queue")
	}
	if len(l.queue) != 1 {
		t.Fatalf("queue holds %d, want 1", len(l.queue))
	}
}

func TestTraceIsTheCallSite(t *testing.T) {
	drain := use(t, Config{Ring: RingConfig{Level: slog.LevelDebug, Size: 10}})

	Warn(Log{Message: "where"}) // this line
	drain()

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
