// log.go

package logger

import "log/slog"

// payload is what the five logging functions accept: Log and BetLog only.
type payload interface {
	message() string
	attrs() []slog.Attr
	fill(*Record)
}

// slog/stderr attrs
func appendRaceAttrs(a []slog.Attr, r *RaceDetails) []slog.Attr {
	if r == nil {
		return a
	}
	a = appendStr(a, "venue", r.Venue)
	a = appendInt(a, "race", r.RaceNumber)
	a = appendInt(a, "runner", r.RunnerNumber)
	return appendStr(a, "runner_name", r.RunnerName)
}

func appendStr(a []slog.Attr, k, v string) []slog.Attr {
	if v == "" {
		return a
	}
	return append(a, slog.String(k, v))
}

func appendInt(a []slog.Attr, k string, v int) []slog.Attr {
	if v == 0 {
		return a
	}
	return append(a, slog.Int(k, v))
}

func appendFloat(a []slog.Attr, k string, v float64) []slog.Attr {
	if v == 0 {
		return a
	}
	return append(a, slog.Float64(k, v))
}
