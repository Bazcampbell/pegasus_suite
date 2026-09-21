// packages/tpd/gmax.go

package tpd

import (
	"context"
	"fmt"
	"pegasus_suite/apps/pegasus/core"
	"sync"
	"time"

	"github.com/Bazcampbell/goreq"
	logger "pegasus_suite/logger"
)

const (
	raceListURL     = "https://www.gmaxequine.com/TPD/client/racelist.ashx"
	raceListRefresh = 2 * time.Hour
	raceListTimeout = 10 * time.Second

	prefetchDays = 1
	retainDays   = 2

	// past this with no successful fetch, no race resolves, warn
	staleAfter = 30 * time.Minute
)

// caches upcoming race list
type GmaxClient struct {
	licenceKey string

	mu          sync.RWMutex
	races       map[string]Race
	loaded      map[string]bool
	lastSuccess time.Time

	wanted chan string
}

func NewGmaxClient(licenceKey string) *GmaxClient {
	return &GmaxClient{
		licenceKey: licenceKey,
		races:      make(map[string]Race),
		loaded:     make(map[string]bool),
		wanted:     make(chan string, 8),
	}
}

// returns a race due to start within window
func (l *GmaxClient) RaceDueWithin(window time.Duration) *Race {
	now := time.Now()
	earliest, latest := now.Add(-window), now.Add(window)

	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, race := range l.races {
		if !race.Published || race.PostTime.IsZero() {
			continue
		}
		if race.PostTime.Before(earliest) || race.PostTime.After(latest) {
			continue
		}
		return &race
	}

	return nil
}

func (l *GmaxClient) Lookup(sharecode string) (Race, bool) {
	l.mu.RLock()
	race, ok := l.races[sharecode]
	l.mu.RUnlock()

	if ok {
		return race, true
	}

	if len(sharecode) == sharecodeLen {
		select {
		case l.wanted <- sharecode[2:10]:
			logger.Debug(logger.Log{
				Application:      core.AppName,
				FormattedMessage: fmt.Sprintf("tpd race list miss; queued a fetch sharecode=%v date=%v", sharecode, sharecode[2:10]),
			})
		default:
		}
	}

	return Race{}, false
}

// loads synchronously first up to return early on error
// then kicks off the timer to keep tracks fresh
func (l *GmaxClient) run(ctx context.Context) error {
	if err := l.refreshAll(ctx); err != nil {
		return err
	}

	go l.refreshLoop(ctx)

	return nil
}

func (l *GmaxClient) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(raceListRefresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case date := <-l.wanted:
			l.load(date)

		case <-ticker.C:
			l.refreshAll(ctx)
			l.checkStale()
		}
	}
}

func (l *GmaxClient) refreshAll(ctx context.Context) error {
	now := time.Now().UTC()

	dates := make(map[string]bool)
	for offset := -prefetchDays; offset <= prefetchDays; offset++ {
		dates[now.AddDate(0, 0, offset).Format("20060102")] = true
	}
	for _, date := range l.loadedDates() {
		dates[date] = true
	}

	oldest := now.AddDate(0, 0, -retainDays).Format("20060102")

	var loaded int
	var lastErr error

	for date := range dates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if date < oldest {
			l.drop(date)
			continue
		}
		if err := l.load(date); err != nil {
			lastErr = err
			continue
		}
		loaded++
	}

	// one date failing is fine (future card not populated yet)
	// none loading is cause for error
	if loaded == 0 && lastErr != nil {
		return lastErr
	}

	return nil
}

func (l *GmaxClient) load(date string) error {
	err := l.fetch(date)
	if err != nil {
		logger.Warn(logger.Log{
			Application:      core.AppName,
			FormattedMessage: fmt.Sprintf("tpd race list fetch failed error=%v", err),
		})
	}
	return err
}

func (l *GmaxClient) checkStale() {
	l.mu.RLock()
	last := l.lastSuccess
	l.mu.RUnlock()

	if !last.IsZero() && time.Since(last) < staleAfter {
		return
	}

	logger.Error(logger.Log{
		Application:      core.AppName,
		FormattedMessage: fmt.Sprintf("tpd race list stale; no race will resolve and no bet will place last_success=%v", last),
	})
}

func (l *GmaxClient) drop(date string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.loaded, date)
	for sharecode := range l.races {
		if len(sharecode) == sharecodeLen && sharecode[2:10] == date {
			delete(l.races, sharecode)
		}
	}
}

func (l *GmaxClient) loadedDates() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	dates := make([]string, 0, len(l.loaded))
	for date := range l.loaded {
		dates = append(dates, date)
	}
	return dates
}

// replaces every entry for one date
func (l *GmaxClient) fetch(dateCompact string) error {
	races, err := l.get(dateCompact)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, race := range races {
		l.races[string(race.ID)] = race
	}
	l.loaded[dateCompact] = true
	l.lastSuccess = time.Now()

	return nil
}

func (l *GmaxClient) get(dateCompact string) ([]Race, error) {
	if len(dateCompact) != 8 {
		return nil, fmt.Errorf("tpd: bad race list date %q", dateCompact)
	}

	dateLocal := dateCompact[:4] + "-" + dateCompact[4:6] + "-" + dateCompact[6:]

	races, err := goreq.GetType[[]Race](raceListURL, &goreq.Options{
		Query:   map[string]string{"DateLocal": dateLocal, "k": l.licenceKey},
		Timeout: raceListTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("tpd: race list %s: %w", dateLocal, err)
	}

	return races, nil
}
