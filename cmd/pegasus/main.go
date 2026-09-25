// cmd/pegasus/main.go
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
	"pegasus_suite/logger/telegram"
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

	log      logger.Config
	telegram telegram.Config
}

func mustLevel(key string) slog.Level {
	level, err := logger.ParseLevel(util.MustEnv(key))
	if err != nil {
		slog.Error("bad "+key, "error", err)
		os.Exit(1)
	}
	return level
}

func loadEnv() env {
	return env{
		adminUserID: util.MustEnv("ADMIN_USER_ID"),
		settingsURL: util.MustEnv("SETTINGS_URL"),

		port: util.MustEnv("WAGERING_PORT"),
		// SESSION sets no aud claim, so there is no audience to check.
		auth: auth.Config{
			URL:    util.MustEnv("JWK_URL"),
			Issuer: util.MustEnv("JWT_ISSUER"),
		},

		log: logger.Config{
			Application:   application,
			DefaultUserID: util.MustEnv("ADMIN_USER_ID"),
			StdErrLevel:   mustLevel("LOG_LEVEL"),
			Ring: logger.RingConfig{
				Level: mustLevel("LOG_RING_LEVEL"),
				Size:  int(util.MustEnvInt64("LOG_RING_SIZE")),
			},
		},
		telegram: telegram.Config{
			Level:        mustLevel("LOG_TG_LEVEL"),
			BotToken:     util.MustEnv("LOG_TG_BOT_TOKEN"),
			LogChannelID: util.MustEnvInt64("LOG_TG_CHANNEL_ID"),
			BetChannelID: util.MustEnvInt64("LOG_TG_BET_CHANNEL_ID"),
			QueueSize:    int(util.MustEnvInt64("LOG_TG_QUEUE_SIZE")),
			DedupeWindow: time.Duration(util.MustEnvInt64("LOG_TG_DEDUPE_MS")) * time.Millisecond,
		},
	}
}

// brings up the kernel and API only
// runtime is managed by admin page
func main() {
	_ = godotenv.Load()
	cfg := loadEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// settings as JSON files in a bucket
	// s3 or local dir
	bucket, err := store.Open(ctx, cfg.settingsURL)
	if err != nil {
		slog.Error("unable to open settings bucket", "error", err)
		os.Exit(1)
	}

	// Telegram is optional: without it the logger still runs, stderr and ring.
	var sinks []logger.Sink
	if tg, err := telegram.New(cfg.telegram); err != nil {
		slog.Warn("telegram logging unavailable", "error", err)
	} else {
		sinks = append(sinks, tg)
	}
	logger.Init(cfg.log, sinks...)
	defer logger.Stop()

	store := doc.New(bucket)

	k := kernel.New(store)

	// register applications
	k.Register(pegasus.New())

	defer k.Stop()

	server, err := api.NewServer(cfg.port, cfg.auth, k)
	if err != nil {
		slog.Error("unable to initialise api server", "error", err)
		os.Exit(1)
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Run()
	}()

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
