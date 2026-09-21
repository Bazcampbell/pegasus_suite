// config.go

package logger

import (
	"fmt"
	"log/slog"
	"strings"
)

const LevelBet slog.Level = 2

// core logger, stderr and in-mem ring
type Config struct {
	Application   string // used if log has no app value
	DefaultUserID string

	StdErrLevel slog.Level
	Ring        RingConfig
}

type RingConfig struct {
	Level slog.Level
	Size  int // how many entries are kept
}

const defaultRingSize = 10000

func (r RingConfig) size() int {
	if r.Size <= 0 {
		return defaultRingSize
	}
	return r.Size
}

func ParseLevel(v string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "bet":
		return LevelBet, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("logger: unknown level %q", v)
}

func LevelName(l slog.Level) string {
	if l == LevelBet {
		return "BET"
	}
	return l.String()
}
