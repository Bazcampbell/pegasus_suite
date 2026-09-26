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
	ProcessID   string          `json:"process_id,omitempty"`
	Race        *Race           `json:"race,omitempty"`
	Trace       json.RawMessage `json:"trace,omitempty"` // {package, function, file, line}
	Request     json.RawMessage `json:"request,omitempty"`
	Response    json.RawMessage `json:"response,omitempty"`

	// set only for a BetLog; bets never reach the ring
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
	r.Trace = traceJSON(pc)

	if r.UserID == "" {
		r.UserID = defaultUserID
	}
	return r
}

// traceJSON returns the call site as {package, function, file, line}.
func traceJSON(pc uintptr) json.RawMessage {
	m := make(map[string]any, 4)
	if pc != 0 {
		if f, _ := runtime.CallersFrames([]uintptr{pc}).Next(); f.Function != "" || f.File != "" {
			m["package"], m["function"] = splitFuncName(f.Function)
			m["file"] = f.File
			m["line"] = f.Line
		}
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
