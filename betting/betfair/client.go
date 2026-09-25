// betfair/client.go

package betfair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
	"pegasus_suite/logger"
)

const (
	TOKEN_REFRESH_INTERVAL = 5 * time.Hour
	TRACK_REFRESH_INTERVAL = 1 * time.Hour
)

type Client struct {
	api *exchange.Client

	cancelToken context.CancelFunc

	// country:track → event
	mu           sync.RWMutex
	thoroughbred map[string]*Event
	trot         map[string]*Event

	// market ID → map[selection ID]RunnerPrices, written only by the stream goroutine
	prices sync.Map

	// the stream's next subscription; a newer set replaces an unread one
	subscription chan []string
}

func NewBetfairClient(username, password, appKey, cert string) (*Client, error) {
	api, err := exchange.New(username, password, appKey, cert)
	if err != nil {
		return nil, err
	}
	return &Client{
		api:          api,
		thoroughbred: make(map[string]*Event),
		trot:         make(map[string]*Event),
		subscription: make(chan []string, 1),
	}, nil
}

func (bc *Client) Provider() betting.Provider { return betting.ProviderBetfair }

func (bc *Client) Account() string { return bc.api.Username }

// Close stops the token refresh and logs out. Loops started with a context stop with it.
func (bc *Client) Close() {
	if bc.cancelToken != nil {
		bc.cancelToken()
	}
	if err := bc.api.Logout(); err != nil {
		logger.Warn(logger.Log{Message: fmt.Sprintf("betfair logout failed account=%s error=%v", bc.api.Username, err)})
	}
}

func (bc *Client) RefreshToken() error { return bc.api.KeepAlive() }

// StartTokenRefresh keeps the session alive every TOKEN_REFRESH_INTERVAL until Close or ctx ends.
func (bc *Client) StartTokenRefresh(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	bc.cancelToken = cancel
	every(ctx, TOKEN_REFRESH_INTERVAL, func() {
		if err := bc.RefreshToken(); err != nil {
			logger.Warn(logger.Log{Message: fmt.Sprintf("betfair token refresh failed account=%s error=%v", bc.api.Username, err)})
		}
	})
}

// StartTrackRefresh reloads the race catalogue for countries every TRACK_REFRESH_INTERVAL until ctx ends.
func (bc *Client) StartTrackRefresh(ctx context.Context, countries []string) {
	every(ctx, TRACK_REFRESH_INTERVAL, func() { bc.loadUpcomingEvents(countries) })
}

// every runs fn now and then every interval, on its own goroutine, until ctx ends.
func every(ctx context.Context, interval time.Duration, fn func()) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			fn()
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}

var _ betting.Client = (*Client)(nil)
