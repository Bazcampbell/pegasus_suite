// config.go

package logger

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Config struct {
	Application   string
	DefaultUserID string // sent when no user ID is provided

	Level slog.Level

	Telegram *TelegramSetup // optional

	Setup LoggerSetup
}

type TelegramSetup struct {
	BotToken string

	BetChannelID *int64
	LogChannelID *int64
}

func (t *TelegramSetup) channelFor(logType string) *int64 {
	if logType == "BET" {
		return t.BetChannelID
	} else {
		return t.LogChannelID
	}
}

func (t *TelegramSetup) enabled() bool {
	return t != nil && t.BotToken != "" &&
		(t.LogChannelID != nil || t.BetChannelID != nil)
}

type LoggerSetup struct {
	// how many recent entries the live view keeps in memory
	RingSize int

	// bounds the Telegram sink channel
	// full queue drops the record
	TelegramQueueSize int

	// collapses identical repeated messages sent to Telegram
	// within this window into a single "+N more" summary.
	// zero disables
	DedupeWindow time.Duration
}

const (
	defaultRingSize  = 10000
	defaultQueueSize = 1000
)

// withDefaults guards against a zero-sized ring or queue, which would panic
// or drop everything. DedupeWindow is left alone: zero means disabled.
func (s LoggerSetup) withDefaults() LoggerSetup {
	if s.RingSize <= 0 {
		s.RingSize = defaultRingSize
	}
	if s.TelegramQueueSize <= 0 {
		s.TelegramQueueSize = defaultQueueSize
	}
	return s
}

// ParseLevel maps debug, info, bet, warn/warning or error to a level.
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
