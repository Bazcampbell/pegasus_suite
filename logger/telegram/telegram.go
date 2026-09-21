// logger/telegram/telegram.go
//
// A logger sink that posts to Telegram
// bets and logs to seperate channels
//
// one goroutine drains a bounded queue and sends in order
// full queue drops the line
//
// at most 18 messages a minute into a channel (bursts of 18),
// under Telegram's ~20; a 429 is waited out for as long as it asks
// dedupe: identical lines within DedupeWindow go out once, followed by a
// "+N more" summary when the window closes.

package telegram

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"pegasus_suite/logger"
)

type Config struct {
	Level slog.Level

	BotToken     string
	LogChannelID int64
	BetChannelID int64

	QueueSize    int
	DedupeWindow time.Duration // 0 disables
}

const (
	defaultQueueSize = 1000
	channelRate      = 18.0 / 60 // messages a second into one channel
	channelBurst     = 18.0
	sendTimeout      = 10 * time.Second
)

type Sink struct {
	cfg    Config
	client *client
	queue  chan logger.Record

	// owned by the run goroutine
	suppressed map[string]*suppression
	pace       map[int64]*bucket

	now   func() time.Time
	sleep func(time.Duration)

	wg       sync.WaitGroup
	stopOnce sync.Once
}

type suppression struct {
	firstSent time.Time
	count     int
	channelID int64
	sample    logger.Record
}

type bucket struct {
	tokens float64
	last   time.Time
}

func New(cfg Config) (*Sink, error) {
	if cfg.BotToken == "" {
		return nil, errors.New("telegram: no bot token")
	}
	if cfg.LogChannelID == 0 && cfg.BetChannelID == 0 {
		return nil, errors.New("telegram: no channel")
	}
	c := &client{baseURL: "https://api.telegram.org", token: cfg.BotToken, http: &http.Client{Timeout: sendTimeout}}
	if err := c.check(); err != nil {
		return nil, err
	}
	s := newSink(cfg, c)
	s.start()
	return s, nil
}

func newSink(cfg Config, c *client) *Sink {
	size := cfg.QueueSize
	if size <= 0 {
		size = defaultQueueSize
	}
	s := &Sink{
		cfg:        cfg,
		client:     c,
		queue:      make(chan logger.Record, size),
		suppressed: map[string]*suppression{},
		pace:       map[int64]*bucket{},
		now:        time.Now,
		sleep:      time.Sleep,
	}
	return s
}

func (s *Sink) start() {
	s.wg.Add(1)
	go s.run()
}

func (s *Sink) Enabled(level slog.Level) bool {
	if level == logger.LevelBet {
		return s.cfg.BetChannelID != 0
	}
	return s.cfg.LogChannelID != 0 && level >= s.cfg.Level
}

func (s *Sink) Enqueue(r logger.Record) {
	select {
	case s.queue <- r:
	default: // never block the app on Telegram
	}
}

// sends what's queued, flushes any "+N more" summaries, returns
func (s *Sink) Stop() {
	s.stopOnce.Do(func() {
		close(s.queue)
		s.wg.Wait()
	})
}

func (s *Sink) channelFor(level string) int64 {
	if level == "BET" {
		return s.cfg.BetChannelID
	}
	return s.cfg.LogChannelID
}

func (s *Sink) run() {
	defer s.wg.Done()

	// the sweep only bounds how soon a "+N more" lands; a quarter of the
	// window is prompt enough without busy-waking
	every := s.cfg.DedupeWindow / 4
	if every <= 0 {
		every = time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()

	for {
		select {
		case r, ok := <-s.queue:
			if !ok {
				s.sweep(true)
				return
			}
			s.dispatch(r)
		case <-tick.C:
			s.sweep(false)
		}
	}
}

// sends a record, unless an identical one is inside dedupe window
func (s *Sink) dispatch(r logger.Record) {
	ch := s.channelFor(r.Level)
	if ch == 0 {
		return
	}
	if s.cfg.DedupeWindow > 0 {
		key := r.Level + "\x00" + r.Message + "\x00" + r.ProcessID
		now := s.now()
		if sup, ok := s.suppressed[key]; ok && now.Sub(sup.firstSent) < s.cfg.DedupeWindow {
			sup.count++
			return
		}
		s.suppressed[key] = &suppression{firstSent: now, channelID: ch, sample: r}
	}
	s.send(ch, formatTelegram(r))
}

func (s *Sink) sweep(force bool) {
	now := s.now()
	for key, sup := range s.suppressed {
		if !force && now.Sub(sup.firstSent) < s.cfg.DedupeWindow {
			continue
		}
		if sup.count > 0 {
			s.send(sup.channelID, formatSuppressed(sup.sample, sup.count))
		}
		delete(s.suppressed, key)
	}
}

func (s *Sink) send(ch int64, text string) {
	for attempt := 0; attempt < 2; attempt++ {
		s.waitTurn(ch)

		err := s.client.send(ch, text)
		if err == nil {
			return
		}

		var ra RetryAfterError
		if errors.As(err, &ra) && attempt == 0 {
			s.pace[ch].tokens = 0
			s.sleep(ra.After)
			continue
		}
		// stderr, not the logger: a logged error would come straight back into the sink and loop
		fmt.Fprintf(os.Stderr, "logger: telegram send failed (channel %d): %v\n", ch, err)
		return
	}
}

// blocks until the channel's bucket has a token, then takes it.
func (s *Sink) waitTurn(ch int64) {
	b := s.pace[ch]
	if b == nil {
		b = &bucket{tokens: channelBurst, last: s.now()}
		s.pace[ch] = b
	}
	for {
		now := s.now()
		b.tokens = min(channelBurst, b.tokens+now.Sub(b.last).Seconds()*channelRate)
		b.last = now
		// a hair under 1 after a timed wait is float rounding, not a missing token
		if b.tokens >= 1-1e-6 {
			b.tokens = max(b.tokens-1, 0)
			return
		}
		// round up, never zero: a wait that rounds down makes no progress
		wait := time.Duration(math.Ceil((1 - b.tokens) / channelRate * float64(time.Second)))
		s.sleep(max(wait, time.Millisecond))
	}
}
