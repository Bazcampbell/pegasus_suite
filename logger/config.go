// config.go

package logger

import (
	"fmt"
	"log/slog"
	"strings"
)

const LevelBet slog.Level = 2

type Config struct {
	Application   string // used when a line names no app
	DefaultUserID string

	StdErrLevel slog.Level
	Ring        RingConfig
	QueueSize   int // lines waiting to be written; 0 means defaultQueueSize
}

type RingConfig struct {
	Level slog.Level
	Size  int // 0 disables the ring
}

func (c Config) queueSize() int {
	if c.QueueSize <= 0 {
		return defaultQueueSize
	}
	return c.QueueSize
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
