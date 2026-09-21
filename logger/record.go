// record.go
//
// the final logged line handed to every sink
// record is filled in with time, level, fallbacks, call site, etc

package logger

import (
	"encoding/json"
	"log/slog"
	"runtime"
	"time"
)

type Record struct {
	Time        time.Time       `json:"time"`
	Level       string          `json:"level"` // DEBUG, INFO, BET, WARN, ERROR
	Application string          `json:"application"`
	Message     string          `json:"message"`
	UserID      string          `json:"user_id,omitempty"`
	Username    string          `json:"username,omitempty"`
	ProcessID   string          `json:"process_id,omitempty"`
	Race        *RaceDetails    `json:"race,omitempty"`
	Trace       json.RawMessage `json:"trace,omitempty"` // {package, function, file, line[, trace]}
	Request     json.RawMessage `json:"request,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`

	// the bet itself, set only for BetLog, for sinks that render it
	// never reaches the ring, so never served.
	Bet *BetLog `json:"-"`
}

func newRecord(p payload, level slog.Level, at time.Time, pc uintptr, application, defaultUserID string) Record {
	r := Record{
		Time:        at,
		Level:       LevelName(level),
		Application: application,
		Message:     p.message(),
	}
	p.fill(&r)

	var extra *string
	if l, ok := p.(Log); ok {
		extra = l.Trace
	}
	r.Trace = traceJSON(pc, extra)

	if r.UserID == "" {
		r.UserID = defaultUserID
	}
	return r
}

// call site as {package, function, file, line} + trace
func traceJSON(pc uintptr, extra *string) json.RawMessage {
	m := make(map[string]any, 5)
	if pc != 0 {
		if f, _ := runtime.CallersFrames([]uintptr{pc}).Next(); f.Function != "" || f.File != "" {
			m["package"], m["function"] = splitFuncName(f.Function)
			m["file"] = f.File
			m["line"] = f.Line
		}
	}
	if extra != nil && *extra != "" {
		m["trace"] = *extra
	}
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// pegasus_suite/engine.(*Engine).Place -> pkg path + function
func splitFuncName(name string) (pkg, fn string) {
	pkgPath, tail := "", name
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			pkgPath, tail = name[:i+1], name[i+1:]
			break
		}
	}
	for i := 0; i < len(tail); i++ {
		if tail[i] == '.' {
			return pkgPath + tail[:i], tail[i+1:]
		}
	}
	return pkgPath + tail, ""
}

func rawJSON(v any) json.RawMessage {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		if json.Valid([]byte(x)) {
			return json.RawMessage(x)
		}
	case []byte:
		if len(x) == 0 {
			return nil
		}
		if json.Valid(x) {
			return append(json.RawMessage(nil), x...)
		}
		v = string(x)
	case json.RawMessage:
		if len(x) == 0 {
			return nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
