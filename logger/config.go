// config.go

package logger

import "time"

type Config struct {
	Application   string
	DefaultUserID string // sent when no user ID is provided

	Telegram *TelegramSetup // optional

	Setup LoggerSetup
}

type TelegramSetup struct {
	BotToken string

	InfoLogChannelID  *int64
	WarnLogChannelID  *int64
	ErrorLogChannelID *int64
	BetLogChannelID   *int64

	DefaultChannelID *int64 // if above channels not set, default here
}

func (t *TelegramSetup) channelFor(logType string) *int64 {
	var id *int64
	switch logType {
	case "INFO":
		id = t.InfoLogChannelID
	case "WARN":
		id = t.WarnLogChannelID
	case "ERROR":
		id = t.ErrorLogChannelID
	case "BET":
		id = t.BetLogChannelID
	default:
		return nil
	}
	if id == nil {
		id = t.DefaultChannelID
	}
	return id
}

func (t *TelegramSetup) enabled() bool {
	return t != nil && t.BotToken != "" &&
		(t.InfoLogChannelID != nil || t.WarnLogChannelID != nil ||
			t.ErrorLogChannelID != nil || t.BetLogChannelID != nil ||
			t.DefaultChannelID != nil)
}

type LoggerSetup struct {
	// RingSize is how many recent entries the live view keeps in memory.
	RingSize int

	// TelegramQueueSize bounds the Telegram sink's channel. Full queue drops
	// the record rather than block the application.
	TelegramQueueSize int
	// DedupeWindow collapses identical repeated messages sent to Telegram
	// within this window into a single "+N more" summary. Zero disables.
	DedupeWindow time.Duration
}

const (
	defaultRingSize     = 10000
	defaultQueueSize    = 1000
	defaultDedupeWindow = 60 * time.Second
)

func (s LoggerSetup) withDefaults() LoggerSetup {
	if s.RingSize <= 0 {
		s.RingSize = envInt("LOG_RING_SIZE", defaultRingSize)
	}
	if s.TelegramQueueSize <= 0 {
		s.TelegramQueueSize = envInt("LOG_TG_QUEUE_SIZE", defaultQueueSize)
	}
	if s.DedupeWindow == 0 {
		ms := envInt("LOG_TG_DEDUPE_MS", int(defaultDedupeWindow/time.Millisecond))
		s.DedupeWindow = time.Duration(ms) * time.Millisecond
	}
	return s
}
