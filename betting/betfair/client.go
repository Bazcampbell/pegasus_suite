// betfair/client.go

package betfair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pegasus_suite/betting/betfair/internal/exchange"

	"pegasus_suite/betting"
	"pegasus_suite/logger"
)

const (
	ORDER_BOOK_DEPTH       = 3
	RUNNER_UPDATE_INTERVAL = 1 * time.Second
	TOKEN_REFRESH_INTERVAL = 5 * time.Hour
	TRACK_REFRESH_INTERVAL = 1 * time.Hour
)

type Client struct {
	api *exchange.Client

	cancelToken context.CancelFunc
	cancelTrack context.CancelFunc

	// country:track
	mu                         sync.RWMutex
	upcomingThoroughbredEvents map[string]*Event
	upcomingTrotEvents         map[string]*Event

	runnerMu      sync.Mutex
	runnerCancels map[string]context.CancelFunc
}

func NewBetfairClient(username, password, appKey, cert string) (*Client, error) {
	api, err := exchange.New(username, password, appKey, cert)
	if err != nil {
		return nil, err
	}

	return &Client{
		api:                        api,
		upcomingThoroughbredEvents: make(map[string]*Event),
		upcomingTrotEvents:         make(map[string]*Event),
		runnerCancels:              make(map[string]context.CancelFunc),
	}, nil
}

func (bc *Client) Provider() betting.Provider { return betting.ProviderBetfair }

func (bc *Client) Account() string { return bc.api.Username }

func (bc *Client) Close() {
	if bc.cancelToken != nil {
		bc.cancelToken()
	}
	if bc.cancelTrack != nil {
		bc.cancelTrack()
	}

	bc.stopAllRunnerUpdates()

	if err := bc.api.Logout(); err != nil {
		logger.Warn(logger.Log{
			Message: fmt.Sprintf("betfair logout failed error=%v", err),
			Request: bc.api.Username,
		})
	}
}

func (bc *Client) RefreshToken() error { return bc.api.KeepAlive() }

func (bc *Client) StartTokenRefresh(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	bc.cancelToken = cancel

	go func() {
		ticker := time.NewTicker(TOKEN_REFRESH_INTERVAL)
		defer ticker.Stop()

		bc.tickRefresh()

		for {
			select {
			case <-ticker.C:
				bc.tickRefresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (bc *Client) tickRefresh() {
	if err := bc.RefreshToken(); err != nil {
		logger.Warn(logger.Log{
			Message: fmt.Sprintf("betfair token refresh failed error=%v", err),
			Request: bc.api.Username,
		})
		return
	}
}

func (bc *Client) StartTrackRefresh(parent context.Context, countryCodes []string) {
	ctx, cancel := context.WithCancel(parent)
	bc.cancelTrack = cancel

	go func() {
		ticker := time.NewTicker(TRACK_REFRESH_INTERVAL)
		defer ticker.Stop()

		bc.loadUpcomingEvents(countryCodes)

		for {
			select {
			case <-ticker.C:
				bc.loadUpcomingEvents(countryCodes)
			case <-ctx.Done():
				return
			}
		}
	}()
}

var (
	_ betting.Client     = (*Client)(nil)
	_ betting.BetRequest = BetRequest{}
)
