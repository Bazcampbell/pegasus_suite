// logger/record.go
//
// Record is an Entry shaped for readers: the ring buffer's API and the
// archive's NDJSON lines. JSONB columns become raw JSON, the unexported bet
// payload is dropped.

package logger

import (
	"encoding/json"
	"strings"
	"time"
)

type Record struct {
	Time        time.Time       `json:"time"`
	Level       string          `json:"level"`
	Application string          `json:"application"`
	Message     string          `json:"message"`
	UserID      string          `json:"user_id,omitempty"`
	Username    string          `json:"username,omitempty"`
	ProcessID   string          `json:"process_id,omitempty"`
	Race        *RaceDetails    `json:"race,omitempty"`
	Error       string          `json:"error,omitempty"`
	Trace       json.RawMessage `json:"trace,omitempty"`
	Bet         json.RawMessage `json:"bet,omitempty"`
	Request     json.RawMessage `json:"request,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`
}

func (e Entry) Record() Record {
	return Record{
		Time:        e.Time,
		Level:       e.LogType,
		Application: e.Application,
		Message:     e.Message,
		UserID:      e.UserID,
		Username:    e.Username,
		ProcessID:   e.ProcessID,
		Race:        e.RaceDetails,
		Error:       e.Error,
		Trace:       raw(e.Trace),
		Bet:         raw(e.Bet),
		Request:     raw(e.Request),
		Response:    raw(e.Response),
	}
}

func raw(s *string) json.RawMessage {
	if s == nil || *s == "" {
		return nil
	}
	return json.RawMessage(*s)
}

// Query narrows a read of the ring. Zero values match everything.
type Query struct {
	Level       string // DEBUG, INFO, BET, WARN, ERROR
	Application string
	UserID      string
	ProcessID   string
	Since       time.Time
	Limit       int
}

func (q Query) matches(e Entry) bool {
	if q.Level != "" && !strings.EqualFold(q.Level, e.LogType) {
		return false
	}
	if q.Application != "" && q.Application != e.Application {
		return false
	}
	if q.UserID != "" && q.UserID != e.UserID {
		return false
	}
	if q.ProcessID != "" && q.ProcessID != e.ProcessID {
		return false
	}
	if !q.Since.IsZero() && e.Time.Before(q.Since) {
		return false
	}
	return true
}
