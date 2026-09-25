// betfair/races.go

package betfair

import (
	"context"
	"fmt"

	"strconv"
	"strings"
	"time"

	"pegasus_suite/betting"
	logger "pegasus_suite/logger"

	"pegasus_suite/betting/betfair/internal/exchange"
)

func (bc *Client) setUpcomingEventsMap(events []Event) {
	thoroughbred := make(map[string]*Event, len(events))
	trot := make(map[string]*Event, len(events))
	for _, event := range events {
		key := strings.ToUpper(event.Country) + ":" + betting.NormaliseTrackKey(event.TrackName)
		if event.Code == TROT {
			trot[key] = &event
			continue
		}
		thoroughbred[key] = &event
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.upcomingThoroughbredEvents = thoroughbred
	bc.upcomingTrotEvents = trot
}

func (bc *Client) GetRace(code RacingCode, country, trackName string, raceNumber int) *Race {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	race := bc.raceByKeyLocked(code, country, betting.NormaliseTrackKey(trackName), raceNumber)
	if race == nil {
		return nil
	}

	return cloneRace(race)
}

func (bc *Client) HasRace(code RacingCode, country, trackName string, raceNumber int) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.raceByKeyLocked(code, country, betting.NormaliseTrackKey(trackName), raceNumber) != nil
}

func (bc *Client) getEventsLocked(code RacingCode) map[string]*Event {
	if code == TROT {
		return bc.upcomingTrotEvents
	} else if code == THOROUGHBRED {
		return bc.upcomingThoroughbredEvents
	}
	return nil
}

// caller must hold bc.mu (read or write).
func (bc *Client) raceByKeyLocked(code RacingCode, country, normTrack string, raceNumber int) *Race {
	event, ok := bc.getEventsLocked(code)[strings.ToUpper(country)+":"+normTrack]
	if !ok {
		return nil
	}
	return event.Races[raceNumber]
}

// polls listMarketBook every RUNNER_UPDATE_INTERVAL until context is cancelled
func (bc *Client) StartRunnerUpdates(parent context.Context, code RacingCode, country, trackName string, raceNumber int) {
	normTrack := betting.NormaliseTrackKey(trackName)
	key := runnerKey(code, country, normTrack, raceNumber)

	bc.mu.RLock()
	race := bc.raceByKeyLocked(code, country, normTrack, raceNumber)
	bc.mu.RUnlock()
	if race == nil {
		logger.Warn(logger.Log{
			FormattedMessage: "betfair start runner updates: race not loaded",
			RaceDetails:      &logger.RaceDetails{Venue: trackName, RaceNumber: raceNumber},
		})
		return
	}

	bc.runnerMu.Lock()
	if _, running := bc.runnerCancels[key]; running {
		bc.runnerMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	bc.runnerCancels[key] = cancel
	bc.runnerMu.Unlock()

	logger.Debug(logger.Log{
		FormattedMessage: fmt.Sprintf("betfair runner updates started raceId: %s", race.ID),
		RaceDetails:      &logger.RaceDetails{Venue: trackName, RaceNumber: raceNumber},
	})

	go bc.runnerUpdateLoop(ctx, key, code, country, normTrack, raceNumber)
}

func (bc *Client) StopRunnerUpdates(code RacingCode, country, trackName string, raceNumber int) {
	bc.stopRunnerUpdates(runnerKey(code, country, betting.NormaliseTrackKey(trackName), raceNumber))
}

func (bc *Client) stopRunnerUpdates(key string) {
	bc.runnerMu.Lock()
	cancel, ok := bc.runnerCancels[key]
	delete(bc.runnerCancels, key)
	bc.runnerMu.Unlock()

	if ok {
		cancel()
	}
}

func (bc *Client) stopAllRunnerUpdates() {
	bc.runnerMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(bc.runnerCancels))
	for key, cancel := range bc.runnerCancels {
		cancels = append(cancels, cancel)
		delete(bc.runnerCancels, key)
	}
	bc.runnerMu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (bc *Client) runnerUpdateLoop(ctx context.Context, key string, code RacingCode, country, normTrack string, raceNumber int) {
	ticker := time.NewTicker(RUNNER_UPDATE_INTERVAL)
	defer ticker.Stop()

	// remove cancel func from register whichever way we exit
	// ensures a later start can run again
	defer bc.stopRunnerUpdates(key)

	// initial update
	if closed := bc.updateRunners(code, country, normTrack, raceNumber); closed {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if closed := bc.updateRunners(code, country, normTrack, raceNumber); closed {
				logger.Debug(logger.Log{
					FormattedMessage: "betfair runner updates stopping: market closed",
					RaceDetails:      &logger.RaceDetails{Venue: normTrack, RaceNumber: raceNumber},
				})
				return
			}
		}
	}
}

// fetches latest market book for the race, writes runner LTP + back/lay
// returns true when market is closed
// race is re-resolved each time so track refresh does not interfere
func (bc *Client) updateRunners(code RacingCode, country, normTrack string, raceNumber int) (closed bool) {
	bc.mu.RLock()
	race := bc.raceByKeyLocked(code, country, normTrack, raceNumber)
	var marketID string
	if race != nil {
		marketID = race.ID
	}
	bc.mu.RUnlock()

	if marketID == "" {
		return false
	}

	books, err := bc.listMarketBook(marketID)
	if err != nil {
		logger.Warn(logger.Log{
			FormattedMessage: fmt.Sprintf("betfair runner update failed market_id=%v error=%v", marketID, err),
			RaceDetails:      &logger.RaceDetails{Venue: normTrack, RaceNumber: raceNumber},
		})
		return false
	}
	if len(books) == 0 {
		return false
	}
	book := books[0]

	bc.mu.Lock()
	defer bc.mu.Unlock()

	race = bc.raceByKeyLocked(code, country, normTrack, raceNumber)
	if race == nil {
		return book.Status == exchange.MarketStatusClosed
	}

	// live runners by selection ID to match price feed
	bySelection := make(map[int64]*Runner, len(race.Runners))
	for _, runner := range race.Runners {
		selectionID, err := strconv.ParseInt(runner.SelectionID, 10, 64)
		if err != nil {
			continue
		}
		bySelection[selectionID] = runner
	}

	for _, rb := range book.Runners {
		runner, ok := bySelection[rb.SelectionID]
		if !ok {
			continue
		}
		runner.LTP = rb.LastPriceTraded
		runner.Back = toOrderBook(rb.EX.AvailableToBack)
		runner.Lay = toOrderBook(rb.EX.AvailableToLay)
	}

	return book.Status == exchange.MarketStatusClosed
}
