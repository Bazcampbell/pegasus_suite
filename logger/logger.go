// logger.go
//
// One logging package for the whole suite, fully asynchronous. A log call
// captures its call site and queues the line; one goroutine writes it to
// stderr, the ring and every Sink. A full queue drops the line, so logging
// never holds up the caller.
//
//	logger.Init(logger.Config{...}, telegramSink)
//	defer logger.Stop()
//	logger.Info(logger.Log{App: "pegasus", Message: "feed connected"})

package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Sink is an output beyond stderr and the ring. Enqueue must not block.
type Sink interface {
	Enabled(level slog.Level) bool
	Enqueue(r Record)
	Stop()
}

type Logger struct {
	cfg   Config
	slog  *slog.Logger
	ring  *ring
	sinks []Sink

	// lowest level any output accepts; anything below is dropped at the call
	min slog.Level

	queue chan entry
	done  chan struct{}
	idle  chan struct{}

	stopOnce sync.Once
}

type entry struct {
	level slog.Level
	at    time.Time
	pc    uintptr
	p     payload
}

const defaultQueueSize = 10000

var std atomic.Pointer[Logger]

func init() { std.Store(start(Config{StdErrLevel: slog.LevelInfo})) }

// Init replaces the package logger with one built from cfg and sinks, flushing the previous one.
func Init(cfg Config, sinks ...Sink) {
	var live []Sink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	l := start(cfg, live...)
	if old := std.Swap(l); old != nil {
		old.stop()
	}
	slog.SetDefault(l.slog)
}

// Stop flushes and closes every sink, leaving a stderr-only logger for shutdown lines.
func Stop() {
	if old := std.Swap(start(Config{StdErrLevel: slog.LevelInfo})); old != nil {
		old.stop()
	}
}

// Recent returns the ring's matching records, newest first.
func Recent(q Query) []Record {
	l := std.Load()
	if l.ring == nil {
		return nil
	}
	return l.ring.recent(q)
}

func Debug(v Log)  { std.Load().emit(slog.LevelDebug, v) }
func Info(v Log)   { std.Load().emit(slog.LevelInfo, v) }
func Warn(v Log)   { std.Load().emit(slog.LevelWarn, v) }
func Error(v Log)  { std.Load().emit(slog.LevelError, v) }
func Bet(v BetLog) { std.Load().emit(LevelBet, v) }

func start(cfg Config, sinks ...Sink) *Logger {
	l := &Logger{
		cfg:   cfg,
		slog:  slog.New(newTextHandler(cfg.StdErrLevel)),
		sinks: sinks,
		queue: make(chan entry, cfg.queueSize()),
		done:  make(chan struct{}),
		idle:  make(chan struct{}),
	}
	if cfg.Ring.Size > 0 {
		l.ring = newRing(cfg.Ring.Size)
	}
	l.min = l.lowestLevel()
	go l.run()
	return l
}

func (l *Logger) lowestLevel() slog.Level {
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, LevelBet, slog.LevelWarn, slog.LevelError} {
		if l.wants(level) {
			return level
		}
	}
	return slog.LevelError + 1
}

func (l *Logger) wants(level slog.Level) bool {
	if level >= l.cfg.StdErrLevel || (l.ring != nil && level != LevelBet && level >= l.cfg.Ring.Level) {
		return true
	}
	for _, s := range l.sinks {
		if s.Enabled(level) {
			return true
		}
	}
	return false
}

func (l *Logger) emit(level slog.Level, p payload) {
	if level < l.min {
		return
	}
	var pcs [1]uintptr
	// skips runtime.Callers, emit and the exported level function
	runtime.Callers(3, pcs[:])

	select {
	case l.queue <- entry{level: level, at: time.Now(), pc: pcs[0], p: p}:
	default:
	}
}

func (l *Logger) run() {
	defer close(l.idle)
	for {
		select {
		case e := <-l.queue:
			l.write(e)
		case <-l.done:
			for {
				select {
				case e := <-l.queue:
					l.write(e)
				default:
					return
				}
			}
		}
	}
}

func (l *Logger) write(e entry) {
	ctx := context.Background()
	if l.slog.Enabled(ctx, e.level) {
		r := slog.NewRecord(e.at, e.level, e.p.message(), e.pc)
		r.AddAttrs(e.p.attrs()...)
		_ = l.slog.Handler().Handle(ctx, r)
	}

	toRing := l.ring != nil && e.level != LevelBet && e.level >= l.cfg.Ring.Level
	var sinks []Sink
	for _, s := range l.sinks {
		if s.Enabled(e.level) {
			sinks = append(sinks, s)
		}
	}
	if !toRing && len(sinks) == 0 {
		return
	}

	rec := newRecord(e.p, e.level, e.at, e.pc, l.cfg.Application, l.cfg.DefaultUserID)
	if toRing {
		l.ring.add(rec)
	}
	for _, s := range sinks {
		s.Enqueue(rec)
	}
}

func (l *Logger) stop() {
	l.stopOnce.Do(func() {
		close(l.done)
		<-l.idle
		for _, s := range l.sinks {
			s.Stop()
		}
	})
}

func newTextHandler(level slog.Level) slog.Handler {
	return slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelBet {
					a.Value = slog.StringValue("BET")
				}
			}
			return a
		},
	})
}
