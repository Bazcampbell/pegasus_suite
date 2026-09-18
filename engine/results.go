// engine/results.go
//
// Betmatic sizes and fills a notification itself, so the placement call says
// nothing about what got on. Results polls each label a bet was placed under
// and logs the settled outcome once.

package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"pegasus_suite/betting/betmatic"
	"pegasus_suite/logger"
	"pegasus_suite/platform/util"
)

const (
	resultsTick     = 5 * time.Minute
	resultsPageSize = 100

	// A label with nothing placed against it for this long stops being polled,
	// and a reported notification is forgotten once it can no longer come back
	// in a query window.
	watchTTL    = 6 * time.Hour
	reportedTTL = 24 * time.Hour

	// The window handed to Betmatic. Races settle within the hour, so a day is
	// generous; it exists to bound the response, not to decide what's reported.
	resultsLookback = 24 * time.Hour

	// A runaway page loop against a shared account would be worse than missing
	// a result, so paging stops here.
	maxResultPages = 20
)

type watch struct {
	client    *betmatic.Client
	label     string
	processID string
	lastBet   time.Time
}

type Results struct {
	// Nothing placed before the runtime started is ever reported: on a restart
	// the notifications from earlier in the day would otherwise all be logged
	// a second time.
	startedAt time.Time

	mu       sync.Mutex
	watching map[string]*watch
	reported map[int64]time.Time
}

func NewResults() *Results {
	return &Results{
		startedAt: time.Now(),
		watching:  map[string]*watch{},
		reported:  map[int64]time.Time{},
	}
}

// Watch starts polling label on the account behind client. Calling it again
// for the same label keeps the watch alive.
func (r *Results) Watch(client *betmatic.Client, label, processID string) {
	if client == nil || label == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := strings.ToLower(client.Email) + "\x00" + label
	if w, ok := r.watching[key]; ok {
		w.lastBet = time.Now()
		w.client = client
		return
	}
	r.watching[key] = &watch{client: client, label: label, processID: processID, lastBet: time.Now()}
}

func (r *Results) Run(ctx context.Context) {
	ticker := time.NewTicker(resultsTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sweep()
		}
	}
}

func (r *Results) sweep() {
	for _, w := range r.due() {
		r.sweepLabel(w)
	}
	r.prune()
}

func (r *Results) due() []watch {
	cutoff := time.Now().Add(-watchTTL)

	r.mu.Lock()
	defer r.mu.Unlock()

	var due []watch
	for _, w := range r.watching {
		if w.lastBet.After(cutoff) {
			due = append(due, *w)
		}
	}
	return due
}

func (r *Results) sweepLabel(w watch) {
	from := time.Now().Add(-resultsLookback)
	if from.Before(r.startedAt) {
		from = r.startedAt
	}

	for page := 1; page <= maxResultPages; page++ {
		resp, err := w.client.GetNotifications(betmatic.GetNotificationsRequest{
			Label:           w.label,
			MeetingDateFrom: from.Format("2006-01-02"),
			Ordering:        "-triggered_at",
			Page:            util.FlexInt(page),
			PageSize:        resultsPageSize,
		})
		if err != nil {
			logger.Warn(logger.ErrorLog{
				Message:   fmt.Sprintf("betmatic notification lookup failed account=%s label=%s error=%v", w.client.Email, w.label, err),
				ProcessID: logger.SystemProcessID,
			})
			return
		}

		for _, res := range resp.Results {
			if !res.Resulted() || res.IsCanceled {
				continue
			}
			if res.TriggeredAt.Before(r.startedAt) {
				continue
			}
			if r.claim(res.ID) {
				r.report(w, res)
			}
		}

		if page >= int(resp.TotalPages) {
			return
		}
	}
}

// claim records a notification as reported and says whether this call was the
// one that did it, so a result is logged once however many passes see it.
func (r *Results) claim(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, seen := r.reported[id]; seen {
		return false
	}
	r.reported[id] = time.Now()
	return true
}

func (r *Results) prune() {
	watchCutoff := time.Now().Add(-watchTTL)
	reportedCutoff := time.Now().Add(-reportedTTL)

	r.mu.Lock()
	defer r.mu.Unlock()

	for key, w := range r.watching {
		if w.lastBet.Before(watchCutoff) {
			delete(r.watching, key)
		}
	}
	for id, at := range r.reported {
		if at.Before(reportedCutoff) {
			delete(r.reported, id)
		}
	}
}

func (r *Results) report(w watch, res betmatic.Result) {
	comp := res.Tip.Competition
	selection, _ := strconv.Atoi(res.Tip.Selection)

	race := &logger.RaceDetails{
		Venue:        comp.Name,
		RaceNumber:   int(comp.EventNumber),
		RunnerNumber: selection,
		RunnerName:   comp.RunnerName(selection),
	}

	accepted := float64(res.TotalAccepted)
	profit := float64(res.Profit)

	// Fixed Profit sizes its own stake from a target, so neither its stake (a
	// unit count) nor that target is money we asked to have on — accepted is the
	// only real figure. The other types name a stake outright.
	requested := 0.0
	if res.Typ != betmatic.FIXED_PROFIT {
		requested = float64(res.TotalWager)
		if requested == 0 {
			requested = float64(res.Stake)
		}
	}

	logger.Bet(logger.BetLog{
		Message: fmt.Sprintf("betmatic bet resulted %s R%d runner %s %s %s accepted %s profit %s",
			comp.Name, int(comp.EventNumber), res.Tip.Selection, res.Tip.Market,
			outcome(accepted, profit), money(accepted), money(profit)),
		ProcessID:   w.processID,
		Endpoint:    "BETMATIC",
		BetType:     string(res.Typ),
		Market:      res.Tip.Market,
		Ref:         res.Label,
		Requested:   requested,
		Accepted:    accepted,
		Odds:        float64(res.AverageOdds),
		Profit:      profit,
		Result:      outcome(accepted, profit),
		RaceDetails: race,
		Response: fmt.Sprintf("notification=%d placings=%s result_time=%s min_odds=%.2f",
			res.ID, comp.Result, res.Tip.ResultTime.Format(time.RFC3339), float64(res.Tip.MinOdds)),
	})
}

func outcome(accepted, profit float64) string {
	switch {
	case accepted == 0:
		return ""
	case profit > 0:
		return "WIN"
	case profit < 0:
		return "LOSE"
	}
	return "VOID"
}

func money(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("$%.2f", v)
}
