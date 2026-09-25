// betmatic/client.go

package betmatic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"pegasus_suite/betting"
	"pegasus_suite/logger"
)

var baseURL = "https://betmatic.app/api"

// authenticated Betmatic session
type Client struct {
	token    atomic.Pointer[string]
	Email    string
	password string

	// race num:runner name:event
	eventMap atomic.Pointer[map[int]map[string]Event]

	cancelToken  context.CancelFunc
	cancelEvents context.CancelFunc
}

func NewBetmaticClient(email, password string) (*Client, error) {
	bc := &Client{
		Email:    email,
		password: password,
	}

	if err := bc.authenticate(); err != nil {
		return nil, fmt.Errorf("unable to authenticate, %w", err)
	}

	return bc, nil
}

func (bc *Client) Provider() betting.Provider { return betting.ProviderBetmatic }

func (bc *Client) Account() string { return bc.Email }

func (bc *Client) getToken() string { return *bc.token.Load() }

func (bc *Client) setToken(t string) { bc.token.Store(&t) }

func (bc *Client) Close() {
	if bc.cancelToken != nil {
		bc.cancelToken()
	}
	if bc.cancelEvents != nil {
		bc.cancelEvents()
	}
}

func (bc *Client) StartTokenRefresh(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	bc.cancelToken = cancel

	go func() {
		ticker := time.NewTicker(5 * time.Hour)
		defer ticker.Stop()

		bc.tickTokenRefresh()

		for {
			select {
			case <-ticker.C:
				bc.tickTokenRefresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (bc *Client) tickTokenRefresh() {
	if err := bc.RefreshToken(); err != nil {
		logger.Warn(logger.Log{
			FormattedMessage: fmt.Sprintf("betmatic token refresh failed error=%v", err),
			Request:          bc.Email,
		})
		return
	}
	logger.Debug(logger.Log{
		FormattedMessage: fmt.Sprintf("betmatic token refreshed request=%v", bc.Email),
	})
}

func (bc *Client) GetEventMap() map[int]map[string]Event {
	ptr := bc.eventMap.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

func (bc *Client) UpdateUpcomingEventsMap(racingCode RacingCode, countryCode string) ([]Event, error) {
	events, err := bc.GetUpcomingEvents(racingCode, countryCode)
	if err != nil {
		return nil, err
	}

	m := make(map[int]map[string]Event, len(events))
	for _, e := range events {
		if _, exists := m[e.EventNumber]; !exists {
			m[e.EventNumber] = make(map[string]Event)
		}
		m[e.EventNumber][e.RunnerNames] = e
	}

	bc.eventMap.Store(&m)

	return events, nil
}

func (bc *Client) StartUpcomingEventsRefresh(parent context.Context, racingCode RacingCode, countryCode string) {
	ctx, cancel := context.WithCancel(parent)
	bc.cancelEvents = cancel

	tickRefresh := func() {
		events, err := bc.UpdateUpcomingEventsMap(racingCode, countryCode)
		if err != nil {
			logger.Warn(logger.Log{
				FormattedMessage: fmt.Sprintf("betmatic upcoming events refresh failed for %s %s ,error=%v", countryCode, string(racingCode), err),
			})
			return
		}
		logger.Debug(logger.Log{
			FormattedMessage: fmt.Sprintf("betmatic upcoming events refreshed for %s % sresponse=%v", countryCode, string(racingCode), map[string]any{"event_count": len(events)}),
		})
	}

	go func() {
		ticker := time.NewTicker(3 * time.Hour)
		defer ticker.Stop()

		tickRefresh()

		for {
			select {
			case <-ticker.C:
				tickRefresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (bc *Client) PlaceBet(req betting.BetRequest) (betId string, err error) {
	notification, ok := req.(NotificationRequest)
	if !ok {
		return "", betting.ErrWrongProvider
	}

	return bc.CreateNotification(notification)
}

func (bc *Client) TurnOnBookies(bookIDs []int, botID string) []error {
	return bc.controlBookies(bookIDs, botID, ON)
}

func (bc *Client) TurnOffBookies(bookIDs []int, botID string) []error {
	return bc.controlBookies(bookIDs, botID, OFF)
}

func (bc *Client) controlBookies(bookIDs []int, botID string, command BookieAccountCommand) []error {
	books, err := bc.GetAllBookieAccounts()
	if err != nil {
		return []error{err}
	}

	bookStringMap := make(map[string]struct{}, len(bookIDs))
	for _, bid := range bookIDs {
		if book, ok := BookmakerIcons[bid]; ok {
			bookStringMap[book.Name] = struct{}{}
		}
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	sem := make(chan struct{}, 5)

	for _, b := range books {
		if botID != "" && b.BotID != botID {
			continue
		}
		if _, ok := bookStringMap[b.Bookie]; !ok {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(b BookieAccount) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := bc.controlBookieByID(b.ID, command); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("bookie %s (id %v): %w", b.Bookie, b.ID, err))
				mu.Unlock()
			}
		}(b)
	}

	wg.Wait()
	return errs
}

// The pool in ENGINE holds these behind betting.Client, so a signature drifting
// out of the interface must fail the build here rather than at the call site.
var (
	_ betting.Client     = (*Client)(nil)
	_ betting.BetRequest = NotificationRequest{}
)
