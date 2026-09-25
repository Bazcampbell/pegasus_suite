// logger/types.go

package logger

import "log/slog"

// Race names a race by its Betmatic venue name, and a runner when there is one.
type Race struct {
	Venue      string `json:"venue,omitempty"`
	Number     int    `json:"race,omitempty"`
	Runner     int    `json:"runner,omitempty"`
	RunnerName string `json:"runner_name,omitempty"`
}

// Log is a debug, info, warn or error line.
type Log struct {
	App       string
	UserID    string
	ProcessID string

	Message string
	Race    *Race

	Request  any // marshalled to JSON when the line is written
	Response any
}

func (l Log) message() string { return l.Message }

func (l Log) attrs() []slog.Attr {
	a := make([]slog.Attr, 0, 8)
	a = appendStr(a, "app", l.App)
	a = appendStr(a, "user_id", l.UserID)
	a = appendStr(a, "process_id", l.ProcessID)
	return appendRaceAttrs(a, l.Race)
}

func (l Log) fill(r *Record) {
	if l.App != "" {
		r.Application = l.App
	}
	r.UserID = l.UserID
	r.ProcessID = l.ProcessID
	r.Race = l.Race
	r.Request = rawJSON(l.Request)
	r.Response = rawJSON(l.Response)
}

// BetLog is one bet sent to a provider, for the bets channel.
type BetLog struct {
	App       string
	UserID    string
	ProcessID string

	Message string
	Race    *Race

	BetID    string
	Provider string // betfair, betmatic
	BetType  string
	Stake    float64
	Odds     float64
	Target   float64
}

func (l BetLog) message() string { return l.Message }

func (l BetLog) attrs() []slog.Attr {
	a := make([]slog.Attr, 0, 10)
	a = appendStr(a, "app", l.App)
	a = appendStr(a, "user_id", l.UserID)
	a = appendStr(a, "process_id", l.ProcessID)
	a = appendRaceAttrs(a, l.Race)
	a = appendStr(a, "bet_id", l.BetID)
	a = appendStr(a, "provider", l.Provider)
	a = appendFloat(a, "stake", l.Stake)
	a = appendFloat(a, "odds", l.Odds)
	return appendFloat(a, "target", l.Target)
}

func (l BetLog) fill(r *Record) {
	if l.App != "" {
		r.Application = l.App
	}
	r.UserID = l.UserID
	r.ProcessID = l.ProcessID
	r.Race = l.Race
	bet := l
	r.Bet = &bet
}
