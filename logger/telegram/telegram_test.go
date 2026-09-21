package telegram

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"pegasus_suite/logger"
)

type sent struct {
	Chat int64
	Text string
}

// fakeAPI is a Bot API that records sendMessage calls. The first `throttle`
// sends are answered 429 with retry_after.
type fakeAPI struct {
	mu       sync.Mutex
	sent     []sent
	throttle int
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasSuffix(r.URL.Path, "/sendMessage") || !strings.Contains(r.URL.Path, "/botTOKEN/") {
		w.Write([]byte(`{"ok":false,"description":"wrong path"}`))
		return
	}
	if f.throttle > 0 {
		f.throttle--
		w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
		return
	}
	f.sent = append(f.sent, sent{body.ChatID, body.Text})
	w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
}

func (f *fakeAPI) all() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.sent...)
}

// fakeClock stands in for time: sleeping is recorded and moves the clock on
// by exactly that much, so waits cost nothing and never spin.
type fakeClock struct {
	mu    sync.Mutex
	t     time.Time
	slept []time.Duration
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }

func (c *fakeClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.t = c.t.Add(d)
	if len(c.slept) > 1000 {
		panic("sink is spinning on sleep")
	}
}

func (c *fakeClock) waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.slept...)
}

// newTestSink runs a sink against a fake API on a fake clock.
func newTestSink(t *testing.T, cfg Config, api *fakeAPI) (*Sink, *fakeClock) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(srv.Close)

	cfg.BotToken = "TOKEN"
	s := newSink(cfg, &client{baseURL: srv.URL, token: "TOKEN", http: srv.Client()})
	clock := &fakeClock{t: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)}
	s.now, s.sleep = clock.now, clock.sleep
	s.start()
	return s, clock
}

func TestRoutingAndLevels(t *testing.T) {
	api := &fakeAPI{}
	s, _ := newTestSink(t, Config{Level: slog.LevelWarn, LogChannelID: 100, BetChannelID: 200}, api)

	if s.Enabled(slog.LevelInfo) || !s.Enabled(slog.LevelWarn) || !s.Enabled(logger.LevelBet) {
		t.Fatalf("enabled: info=%v warn=%v bet=%v", s.Enabled(slog.LevelInfo), s.Enabled(slog.LevelWarn), s.Enabled(logger.LevelBet))
	}

	s.Enqueue(logger.Record{Level: "WARN", Message: "slow feed"})
	s.Enqueue(logger.Record{Level: "BET", Message: "bet placed", Bet: &logger.BetLog{Stake: 10}})
	s.Stop()

	got := api.all()
	if len(got) != 2 || got[0].Chat != 100 || got[1].Chat != 200 {
		t.Fatalf("sent %+v: want the warn to the log channel, the bet to the bet channel", got)
	}
	if !strings.Contains(got[1].Text, "💵 Stake $10.00") {
		t.Fatalf("bet went out without its bet card:\n%s", got[1].Text)
	}
}

func TestNoBetChannelMeansNoBets(t *testing.T) {
	s, _ := newTestSink(t, Config{Level: slog.LevelWarn, LogChannelID: 100}, &fakeAPI{})
	defer s.Stop()
	if s.Enabled(logger.LevelBet) {
		t.Fatal("bets enabled with no bet channel")
	}
}

func TestRepeatsCollapseIntoOneSummary(t *testing.T) {
	api := &fakeAPI{}
	s, _ := newTestSink(t, Config{LogChannelID: 100, DedupeWindow: time.Hour}, api)

	for i := 0; i < 5; i++ {
		s.Enqueue(logger.Record{Level: "ERROR", Application: "pegasus", Message: "feed down", ProcessID: "p1"})
	}
	s.Enqueue(logger.Record{Level: "ERROR", Application: "pegasus", Message: "feed down", ProcessID: "p2"}) // another process: its own line
	s.Stop()                                                                                                // closes the window: the summary goes out

	got := api.all()
	if len(got) != 3 {
		t.Fatalf("sent %d messages, want the first, p2's, and one summary: %+v", len(got), got)
	}
	if !strings.Contains(got[2].Text, "🔁 <b>4 more</b> identical to:\nfeed down") {
		t.Fatalf("summary:\n%s", got[2].Text)
	}
}

func TestRetryAfterIsWaitedOut(t *testing.T) {
	api := &fakeAPI{throttle: 1}
	s, clock := newTestSink(t, Config{LogChannelID: 100}, api)

	s.Enqueue(logger.Record{Level: "ERROR", Message: "boom"})
	s.Stop()

	if got := api.all(); len(got) != 1 {
		t.Fatalf("sent %d, want the message delivered on the retry", len(got))
	}
	if w := clock.waits(); len(w) == 0 || w[0] != 7*time.Second {
		t.Fatalf("waited %v, want the 7s Telegram asked for first", w)
	}
}

func TestPacingHoldsAChannelToItsRate(t *testing.T) {
	s, clock := newTestSink(t, Config{LogChannelID: 100}, &fakeAPI{})

	// 18 go out on the burst; the 19th has to wait for the bucket.
	for i := 0; i < 19; i++ {
		s.Enqueue(logger.Record{Level: "ERROR", Message: "m" + string(rune('a'+i))})
	}
	s.Stop()

	w := clock.waits()
	if len(w) != 1 || w[0] < 3*time.Second || w[0] > 4*time.Second {
		t.Fatalf("waited %v, want one wait of ~3.3s (18 a minute) before the 19th", w)
	}
}

func TestEnqueueNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	defer close(block)

	s := newSink(Config{LogChannelID: 100, QueueSize: 2}, &client{baseURL: srv.URL, token: "T", http: srv.Client()})
	s.start()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { // far more than the queue holds, while sends hang
			s.Enqueue(logger.Record{Level: "ERROR", Message: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked on a stuck Telegram")
	}
}

func TestTransportErrorsNeverShowTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := &client{baseURL: url, token: "123456:SECRET-TOKEN", http: &http.Client{Timeout: time.Second}}
	err := c.send(100, "hi")
	if err == nil {
		t.Fatal("send to a closed server succeeded")
	}
	if strings.Contains(err.Error(), "SECRET-TOKEN") {
		t.Fatalf("token leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "<token>") {
		t.Fatalf("expected the redacted URL in %q", err)
	}
}
