// cmd/wagering/main.go
//
// The one binary. Wires the store, the logger, the kernel and every
// application, then serves the control plane. This is the only file that
// imports both the kernel and an application.

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
	"pegasus_suite/platform/blob"
	"pegasus_suite/platform/util"

	"github.com/joho/godotenv"
)

const application = "wagering"

// env is every variable the binary reads. All are required; nothing below
// main touches the environment.
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
	bucket, err := blob.Open(ctx, cfg.settingsURL)
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

	// A start failure is not fatal on purpose: the server still comes up, so
	// bad settings can be fixed in the admin UI and the runtime started from
	// there rather than by redeploying.
	if err := k.Start(); err != nil {
		slog.Error("initial runtime start failed; server is up, start it via /api/system/start", "error", err)
	}
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
