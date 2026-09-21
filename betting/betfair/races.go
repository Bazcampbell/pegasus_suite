// betfair/races.go

package betfair

import (
	"context"
	"fmt"

	"sort"
	"strconv"
	"strings"
	"time"

	logger "pegasus_suite/logger"

	"pegasus_suite/betting/betfair/internal/exchange"
)

func (bc *Client) setEvents(events []Event) {
	thoroughbred := make(map[string]*Event, len(events))
	trot := make(map[string]*Event, len(events))
	for _, event := range events {
		key := strings.ToUpper(event.Country) + ":" + NormaliseTrack(event.TrackName)
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

	race := bc.raceByKeyLocked(code, country, NormaliseTrack(trackName), raceNumber)
	if race == nil {
		return nil
	}

	return cloneRace(race)
}

func (bc *Client) HasRace(code RacingCode, country, trackName string, raceNumber int) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.raceByKeyLocked(code, country, NormaliseTrack(trackName), raceNumber) != nil
}

// caller must hold bc.mu (read or write). Any code that isn't TROT reads the
// thoroughbred store, matching how an event with no trot marker is classified.
func (bc *Client) eventsLocked(code RacingCode) map[string]*Event {
	if code == TROT {
		return bc.upcomingTrotEvents
	}
	return bc.upcomingThoroughbredEvents
}

// caller must hold bc.mu (read or write).
func (bc *Client) raceByKeyLocked(code RacingCode, country, normTrack string, raceNumber int) *Race {
	event, ok := bc.eventsLocked(code)[strings.ToUpper(country)+":"+normTrack]
	if !ok {
		return nil
	}
	return event.Races[raceNumber]
}

func cloneRace(race *Race) *Race {
	out := *race
	out.Runners = make(map[int]*Runner, len(race.Runners))
	for number, runner := range race.Runners {
		r := *runner
		r.Back = append([]OrderBook(nil), runner.Back...)
		r.Lay = append([]OrderBook(nil), runner.Lay...)
		out.Runners[number] = &r
	}
	return &out
}

// country code -> normalised track -> race numbers, which is the shape GetRace
// and StartRunnerUpdates take their arguments in.
func (bc *Client) LoadedTrackRaces(code RacingCode) map[string]map[string][]int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	out := make(map[string]map[string][]int)
	for _, event := range bc.eventsLocked(code) {
		if len(event.Races) == 0 {
			continue
		}

		numbers := make([]int, 0, len(event.Races))
		for number := range event.Races {
			numbers = append(numbers, number)
		}
		sort.Ints(numbers)

		country := strings.ToUpper(event.Country)
		if out[country] == nil {
			out[country] = make(map[string][]int)
		}
		out[country][NormaliseTrack(event.TrackName)] = numbers
	}

	return out
}

// polls listMarketBook every RUNNER_UPDATE_INTERVAL until derived context is cancelled
func (bc *Client) StartRunnerUpdates(parent context.Context, code RacingCode, country, trackName string, raceNumber int) {
	normTrack := NormaliseTrack(trackName)
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
		FormattedMessage: fmt.Sprintf("betfair runner updates started request=raceId: %s", race.ID),
		RaceDetails:      &logger.RaceDetails{Venue: trackName, RaceNumber: raceNumber},
	})

	go bc.runnerUpdateLoop(ctx, key, code, country, normTrack, raceNumber)
}

func (bc *Client) StopRunnerUpdates(code RacingCode, country, trackName string, raceNumber int) {
	bc.stopRunnerUpdates(runnerKey(code, country, NormaliseTrack(trackName), raceNumber))
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

func runnerKey(code RacingCode, country, normTrack string, raceNumber int) string {
	return fmt.Sprintf("%s:%s:%s:%d", code, strings.ToUpper(country), normTrack, raceNumber)
}

func (bc *Client) runnerUpdateLoop(ctx context.Context, key string, code RacingCode, country, normTrack string, raceNumber int) {
	ticker := time.NewTicker(RUNNER_UPDATE_INTERVAL)
	defer ticker.Stop()

	// Ensure the cancel func is removed from the registry whichever way we exit
	// (parent cancel, Stop, or market closed), so a later Start can run again.
	defer bc.stopRunnerUpdates(key)

	// Update immediately so the first prices land without waiting a full tick.
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

// updateRunners fetches the latest market book for the race and writes each
// runner's LTP and back/lay depth. It returns true when the market is closed,
// signalling the loop to stop. The race is re-resolved each tick so an hourly
// track refresh swapping the events map doesn't leave us updating a detached
// race object.
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

	// Index the live runners by selection id so we can match the price feed,
	// which is keyed by selection id rather than cloth number.
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
		runner.Back = toOrderBook(rb.EX.AvailableToBack, true)
		runner.Lay = toOrderBook(rb.EX.AvailableToLay, false)
	}

	return book.Status == exchange.MarketStatusClosed
}
