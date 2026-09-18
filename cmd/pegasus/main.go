// cmd/wagering/main.go
//
// wires the store, the logger, the kernel and every application, then serves API

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pegasus_suite/apps/pegasus"
	"pegasus_suite/clients/doc"
	"pegasus_suite/kernel"
	"pegasus_suite/kernel/api"
	"pegasus_suite/logger"
	"pegasus_suite/platform/auth"
	"pegasus_suite/platform/store"
	"pegasus_suite/platform/util"

	"github.com/joho/godotenv"
)

const application = "pegasus_suite"

type env struct {
	adminUserID string
	settingsURL string

	port string
	auth auth.Config

	logLevel    slog.Level
	logSetup    logger.LoggerSetup
	logTelegram *logger.TelegramSetup
}

func loadEnv() env {
	level, err := logger.ParseLevel(util.MustEnv("LOG_LEVEL"))
	if err != nil {
		slog.Error("bad LOG_LEVEL", "error", err)
		os.Exit(1)
	}

	betChannelID := util.MustEnvInt64("LOG_TG_BET_CHANNEL_ID")
	logChannelID := util.MustEnvInt64("LOG_TG_CHANNEL_ID")

	return env{
		adminUserID: util.MustEnv("ADMIN_USER_ID"),
		settingsURL: util.MustEnv("SETTINGS_URL"),

		port: util.MustEnv("WAGERING_PORT"),
		// SESSION sets no aud claim, so there is no audience to check.
		auth: auth.Config{
			URL:    util.MustEnv("JWK_URL"),
			Issuer: util.MustEnv("JWT_ISSUER"),
		},

		logLevel: level,
		logSetup: logger.LoggerSetup{
			RingSize:          int(util.MustEnvInt64("LOG_RING_SIZE")),
			TelegramQueueSize: int(util.MustEnvInt64("LOG_TG_QUEUE_SIZE")),
			DedupeWindow:      time.Duration(util.MustEnvInt64("LOG_TG_DEDUPE_MS")) * time.Millisecond,
		},
		logTelegram: &logger.TelegramSetup{
			BotToken:     util.MustEnv("LOG_TG_BOT_TOKEN"),
			BetChannelID: &betChannelID,
			LogChannelID: &logChannelID,
		},
	}
}

func main() {
	_ = godotenv.Load()
	cfg := loadEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Settings live as JSON documents in a bucket: a directory for dev
	// (file:///path), S3 for real (s3://bucket/prefix).
	bucket, err := store.Open(ctx, cfg.settingsURL)
	if err != nil {
		slog.Error("unable to open the settings bucket", "error", err)
		os.Exit(1)
	}

	if err := logger.Init(logger.Config{
		Application:   application,
		DefaultUserID: cfg.adminUserID,
		Level:         cfg.logLevel,
		Telegram:      cfg.logTelegram,
		Setup:         cfg.logSetup,
	}); err != nil {
		slog.Warn("telegram logging unavailable", "error", err)
	}
	defer logger.Stop()

	k := kernel.New(doc.New(bucket))
	k.Register(pegasus.New())

	// Boot brings up the kernel and the API only. The runtime (applications,
	// feeds, sessions, restored processes) starts when an admin asks for it at
	// POST /api/system/start, so nothing connects or bets on a deploy by itself.
	// Stop on shutdown is a no-op if it was never started.
	defer k.Stop()

	server, err := api.NewServer(cfg.port, cfg.auth, k)
	if err != nil {
		slog.Error("unable to build api server", "error", err)
		os.Exit(1)
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Run() }()

	select {
	case err := <-serverErr:
		if err != nil {
			slog.Error("server exited with error", "error", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}
