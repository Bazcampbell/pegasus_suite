// triple-s/connection.go

package triples

import (
	"context"
	"fmt"
	"pegasus_suite/apps/pegasus/core"
	"time"

	logger "pegasus_suite/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ClientID        string
	Topics          []string
}

// connectMQTT presigns a fresh WS URL and dials the broker. Paho's built-in
// auto-reconnect is intentionally disabled — the presigned URL expires, so
// we drive reconnects from triples.Client and presign a new URL each time.
func connectMQTT(
	ctx context.Context,
	cfg Config,
	onMessage mqtt.MessageHandler,
	onConnect mqtt.OnConnectHandler,
	onLost mqtt.ConnectionLostHandler,
) (mqtt.Client, error) {
	creds := aws.Credentials{
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
	}

	presignedURL, err := signedWebSocketURL(ctx, cfg.Endpoint, cfg.Region, creds)
	if err != nil {
		return nil, fmt.Errorf("sign url: %w", err)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(presignedURL)
	opts.SetClientID(cfg.ClientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(false)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetDefaultPublishHandler(onMessage)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(onLost)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		return nil, fmt.Errorf("connect timed out")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	logger.Debug(logger.Log{
		Application:      core.AppName,
		FormattedMessage: fmt.Sprintf("triple-s mqtt dial successful endpoint=%v region=%v client_id=%v", cfg.Endpoint, cfg.Region, cfg.ClientID),
	})

	return client, nil
}
