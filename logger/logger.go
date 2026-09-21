// logger.go
//
// One logging package for the whole suite. main hands Init a Config and any
// extra outputs, and from then on anything in the tree logs typed values:
//
//	tg, err := telegram.New(telegram.Config{...}) // an optional Sink
//	logger.Init(logger.Config{
//	    Application:   "pegasus_suite",
//	    DefaultUserID: adminUserID,
//	    StdErrLevel:   slog.LevelDebug,
//	    Ring:          logger.RingConfig{Level: slog.LevelInfo, Size: 10000},
//	}, tg)
//	defer logger.Stop()
//
//	logger.Info(logger.Log{Application: "pegasus", FormattedMessage: "feed connected"})
//	logger.Bet(logger.BetLog{...})
//
// Every line goes to each output whose minimum level it meets:
//   - stderr, at Config.StdErrLevel: synchronous, from process start.
//   - the ring, at Config.Ring.Level: what GET /api/logs reads. Never bets.
//   - each Sink, which decides for itself (Sink.Enabled).
//
// Nothing here reads the environment and nothing here knows about Telegram.

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

// output beyond stderr and ring
// enqueue cannot block
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

	stopOnce sync.Once
}

var std atomic.Pointer[Logger]

// anything logged before Init reaches stderr
func init() { std.Store(bootstrap()) }

func bootstrap() *Logger {
	return &Logger{slog: slog.New(newTextHandler(slog.LevelInfo))}
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

// installs logger project wide
// previous one is stopped if found, stops leaking routines
func Init(cfg Config, sinks ...Sink) {
	l := &Logger{
		cfg:  cfg,
		slog: slog.New(newTextHandler(cfg.StdErrLevel)),
		ring: newRing(cfg.Ring.size()),
	}
	for _, s := range sinks {
		if s != nil {
			l.sinks = append(l.sinks, s)
		}
	}

	if old := std.Swap(l); old != nil {
		old.stop()
	}
	// third-party code logging through plain slog reaches stderr only
	slog.SetDefault(l.slog)
}

// flushes and closes the sinks
// leaves stderr log only so shutdown logs still go through
func Stop() {
	if old := std.Swap(bootstrap()); old != nil {
		old.stop()
	}
}

func (l *Logger) stop() {
	l.stopOnce.Do(func() {
		for _, s := range l.sinks {
			s.Stop()
		}
	})
}

// Recent reads the ring: matching records, newest first.
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

// sends one line to each output that wants it
func (l *Logger) emit(level slog.Level, p payload) {
	ctx := context.Background()

	toStderr := l.slog.Enabled(ctx, level)
	toRing := l.ring != nil && level != LevelBet && level >= l.cfg.Ring.Level
	toSink := false
	for _, s := range l.sinks {
		if s.Enabled(level) {
			toSink = true
			break
		}
	}
	if !toStderr && !toRing && !toSink {
		return
	}

	var pcs [1]uintptr

	// skips emit() and functions above that called it, landing on real call site
	runtime.Callers(3, pcs[:])
	now := time.Now()

	if toStderr {
		r := slog.NewRecord(now, level, p.message(), pcs[0])
		r.AddAttrs(p.attrs()...)
		_ = l.slog.Handler().Handle(ctx, r)
	}
	if !toRing && !toSink {
		return
	}

	rec := newRecord(p, level, now, pcs[0], l.cfg.Application, l.cfg.DefaultUserID)
	if toRing {
		l.ring.add(rec)
	}
	for _, s := range l.sinks {
		if s.Enabled(level) {
			s.Enqueue(rec)
		}
	}
}
