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
	"strconv"
	"syscall"
	"time"

	"racing_wagering/apps/pegasus"
	"racing_wagering/clients/doc"
	"racing_wagering/kernel"
	"racing_wagering/kernel/api"
	"racing_wagering/logger"
	"racing_wagering/platform/blob"
	"racing_wagering/platform/util"

	"github.com/joho/godotenv"
)

const application = "wagering"

func main() {
	_ = godotenv.Load()

	adminUserID := util.MustEnv("ADMIN_USER_ID")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Settings live as JSON documents in a bucket: a directory for dev
	// (file:///path), S3 for real (s3://bucket/prefix).
	bucket, err := blob.Open(ctx, util.MustEnv("SETTINGS_URL"))
	if err != nil {
		slog.Error("unable to open the settings bucket", "error", err)
		os.Exit(1)
	}

	if err := logger.Init(logger.Config{
		Application:   application,
		DefaultUserID: adminUserID,
		Telegram:      telegramFromEnv(),
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

	server, err := api.NewServer(util.MustEnv("WAGERING_PORT"), util.MustEnv("JWK_URL"), k)
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

// telegramFromEnv is nil, and Telegram logging off, unless a bot token and at
// least one channel are set.
func telegramFromEnv() *logger.TelegramSetup {
	token := os.Getenv("LOG_TG_BOT_TOKEN")
	if token == "" {
		return nil
	}

	return &logger.TelegramSetup{
		BotToken:          token,
		DefaultChannelID:  envChannel("LOG_TG_CHANNEL_ID"),
		InfoLogChannelID:  envChannel("LOG_TG_INFO_CHANNEL_ID"),
		WarnLogChannelID:  envChannel("LOG_TG_WARN_CHANNEL_ID"),
		ErrorLogChannelID: envChannel("LOG_TG_ERROR_CHANNEL_ID"),
		BetLogChannelID:   envChannel("LOG_TG_BET_CHANNEL_ID"),
	}
}

func envChannel(key string) *int64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		slog.Warn("ignoring unparseable telegram channel id", "key", key, "value", v)
		return nil
	}
	return &id
}
