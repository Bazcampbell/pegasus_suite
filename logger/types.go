// logger/types.go

package logger

import "log/slog"

type RaceDetails struct {
	Venue        string `json:"venue,omitempty"` // resolved to the Betmatic name (if known)
	RaceNumber   int    `json:"race,omitempty"`
	RunnerNumber int    `json:"runner,omitempty"`
	RunnerName   string `json:"runner_name,omitempty"`
}

// debug, info, warn, error use
type Log struct {
	Application string
	UserID      string
	Username    string
	ProcessID   string

	FormattedMessage string

	Request  any // marshalled to JSON when the line is recorded
	Response any
	Trace    *string

	RaceDetails *RaceDetails
}

func (l Log) fill(r *Record) {
	if l.Application != "" {
		r.Application = l.Application
	}
	r.UserID = l.UserID
	r.Username = l.Username
	r.ProcessID = l.ProcessID
	r.Race = l.RaceDetails
	r.Request = rawJSON(l.Request)
	r.Response = rawJSON(l.Response)
}

func (l Log) message() string { return l.FormattedMessage }

func (l Log) attrs() []slog.Attr {
	a := make([]slog.Attr, 0, 8)
	a = appendStr(a, "app", l.Application)
	a = appendStr(a, "user_id", l.UserID)
	a = appendStr(a, "username", l.Username)
	a = appendStr(a, "process_id", l.ProcessID)
	return appendRaceAttrs(a, l.RaceDetails)
}

// betting only
type BetLog struct {
	Application string

	UserID    string
	Username  string
	ProcessID string

	RaceDetails *RaceDetails

	Endpoint string // betfair, betmatic, tote
	BetType  string
	Market   string

	// betmatic label or BF customer ref
	Ref string

	Requested float64
	Accepted  float64
	Stake     float64
	Odds      float64
	TargetLia float64
	Profit    float64
	Result    string // WIN, LOSE, VOID, DH

	RaceState string

	Message  string
	Request  any
	Response any
}

// keeps a copy of the bet for sinks
func (l BetLog) fill(r *Record) {
	if l.Application != "" {
		r.Application = l.Application
	}
	r.UserID = l.UserID
	r.Username = l.Username
	r.ProcessID = l.ProcessID
	r.Race = l.RaceDetails

	bet := l
	r.Bet = &bet
}

func (l BetLog) attrs() []slog.Attr {
	a := make([]slog.Attr, 0, 13)
	a = appendStr(a, "app", l.Application)
	a = appendStr(a, "user_id", l.UserID)
	a = appendStr(a, "username", l.Username)
	a = appendStr(a, "process_id", l.ProcessID)
	a = appendRaceAttrs(a, l.RaceDetails)
	a = appendStr(a, "market", l.Market)
	a = appendStr(a, "endpoint", l.Endpoint)
	a = appendFloat(a, "stake", l.Stake)
	a = appendFloat(a, "odds", l.Odds)
	a = appendStr(a, "result", l.Result)
	return a
}

func (l BetLog) message() string { return l.Message }
