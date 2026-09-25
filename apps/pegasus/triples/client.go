// triple-s/client.go

package triples

import (
	"context"
	"encoding/json"
	"fmt"
	"pegasus_suite/apps/pegasus/core"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logger "pegasus_suite/logger"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const subscribeQoS byte = 1

type Handler func(RaceMessage)

type Client struct {
	cfg     Config
	ctx     context.Context
	handler Handler

	onFatal   func(error)
	fatalOnce sync.Once

	// reconnecting guards against overlapping reconnect runs — a rapid flap can
	// fire onConnectionLost repeatedly, and without this each spawns a loop that
	// dials in parallel.
	reconnecting atomic.Bool

	mu   sync.Mutex
	mqtt mqtt.Client
}

func NewClient(ctx context.Context, cfg Config, h Handler, onFatal func(error)) (*Client, error) {
	c := &Client{
		cfg:     cfg,
		ctx:     ctx,
		handler: h,
		onFatal: onFatal,
	}

	mc, err := connectMQTT(ctx, cfg, c.handleMessage, c.onConnect, c.onConnectionLost)
	if err != nil {
		return nil, fmt.Errorf("mqtt client init: %w", err)
	}
	c.mqtt = mc

	if err := c.subscribeAll(mc, c.cfg.Topics); err != nil {
		mc.Disconnect(250)
		return nil, fmt.Errorf("mqtt subscribe: %w", err)
	}

	return c, nil
}

func (c *Client) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	var rm RaceMessage
	if err := json.Unmarshal(msg.Payload(), &rm); err != nil {
		logger.Error(logger.Log{
			App:     core.AppName,
			Message: fmt.Sprintf("triple-s decode failed topic=%v error=%v", msg.Topic(), err),
			Request: msg.Payload(),
		})
		return
	}

	rm.Topic = msg.Topic()
	rm.CountryRaceCode = TopicCode(rm.Topic)

	c.handler(rm)
}

// re-subscribes on every (re)connect because we use CleanSession=true.
// Without this, after a reconnect the broker has no record of our subs and the
// feed goes dark. Subscribes against the freshly-connected client passed in by
// paho — c.mqtt isn't safe to use here: on first connect it's nil (the
// `c.mqtt = mc` assignment in NewClient runs after Connect returns), and on
// reconnect it still points at the old, dead client.
func (c *Client) onConnect(mc mqtt.Client) {
	if err := c.subscribeAll(mc, c.cfg.Topics); err != nil {
		logger.Error(logger.Log{
			App:     core.AppName,
			Message: fmt.Sprintf("triple-s resubscribe failed error=%v", err),
		})
	}
	logger.Info(logger.Log{
		App:     core.AppName,
		Message: fmt.Sprintf("triple-s connected client_id=%v", c.cfg.ClientID),
	})
}

// reconnect loop in a fresh goroutine.
// can't use paho's auto-reconnect because the presigned WS URL is captured at
// dial time and expires; we need to re-sign on every attempt.
func (c *Client) onConnectionLost(_ mqtt.Client, err error) {
	logger.Warn(logger.Log{
		App:     core.AppName,
		Message: fmt.Sprintf("triple-s connection lost error=%v", err),
	})
	go c.reconnectLoop()
}

const maxReconnectAttempts = 5

func (c *Client) reconnectLoop() {
	// Only one reconnect run at a time.
	if !c.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer c.reconnecting.Store(false)

	backoff := time.Second

	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
		}

		logger.Debug(logger.Log{
			App:     core.AppName,
			Message: fmt.Sprintf("triple-s reconnect attempt attempt=%v max_attempts=%v backoff=%v client_id=%v", attempt, maxReconnectAttempts, backoff.String(), c.cfg.ClientID),
		})

		mc, err := connectMQTT(c.ctx, c.cfg, c.handleMessage, c.onConnect, c.onConnectionLost)
		if err != nil {
			logger.Warn(logger.Log{
				App:     core.AppName,
				Message: fmt.Sprintf("triple-s reconnect failed attempt=%v max_attempts=%v client_id=%v error=%v", attempt, maxReconnectAttempts, c.cfg.ClientID, err),
			})
			backoff *= 2
			continue
		}

		c.mu.Lock()
		c.mqtt = mc
		c.mu.Unlock()
		return
	}

	// If we were torn down (stop/restart) during the final attempt's wait/dial,
	// this client is obsolete — don't fire onFatal and stop the runtime that
	// replaced us. The supervisor also generation-guards onFatal; this is the
	// cheap early-out so we don't even log a spurious error.
	select {
	case <-c.ctx.Done():
		return
	default:
	}

	err := fmt.Errorf("triple-s reconnect gave up after %d attempts", maxReconnectAttempts)
	logger.Error(logger.Log{
		App:     core.AppName,
		Message: fmt.Sprintf("triple-s reconnect exhausted; stopping runtime attempts=%v client_id=%v error=%v", maxReconnectAttempts, c.cfg.ClientID, err),
	})
	c.fatalOnce.Do(func() {
		if c.onFatal != nil {
			c.onFatal(err)
		}
	})
}

func (c *Client) subscribeAll(mc mqtt.Client, topics []string) error {
	if mc == nil {
		return fmt.Errorf("no mqtt client")
	}

	filters := make(map[string]byte, len(topics))
	for _, t := range topics {
		filters[t] = subscribeQoS
	}
	token := mc.SubscribeMultiple(filters, nil)
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}

	logger.Info(logger.Log{
		App:     core.AppName,
		Message: fmt.Sprintf("triple-s subscribed topics=%v client_id=%v", strings.Join(topics, ","), c.cfg.ClientID),
	})

	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	mc := c.mqtt
	c.mu.Unlock()
	if mc != nil {
		mc.Disconnect(250)
	}
}
