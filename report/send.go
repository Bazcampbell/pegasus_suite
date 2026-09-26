// report/send.go

package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pegasus_suite/logger"
)

// send posts each user with bets this month a card for yesterday, the last seven days and the month to date.
func (r *Reporter) send(ctx context.Context, yesterday time.Time) {
	week := r.sum(ctx, yesterday.AddDate(0, 0, -6), yesterday)
	month := r.sum(ctx, time.Date(yesterday.Year(), yesterday.Month(), 1, 0, 0, 0, 0, aest), yesterday)
	days := r.sum(ctx, yesterday, yesterday)

	for user, m := range month {
		if m.Attempted == 0 {
			continue
		}
		logger.Bet(logger.BetLog{
			App:    app,
			UserID: user,
			Message: strings.Join([]string{
				"Report for " + yesterday.Format("Mon 2 Jan 2006"),
				line("Yesterday", days[user]),
				line("Last 7 days", week[user]),
				line(yesterday.Format("January"), m),
			}, "\n"),
		})
	}
}

// sum returns each user's totals over the stored days from first to last inclusive; missing days count as nothing.
func (r *Reporter) sum(ctx context.Context, first, last time.Time) map[string]Totals {
	out := make(map[string]Totals)
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		totals, _, err := r.load(ctx, d)
		if err != nil {
			logger.Warn(logger.Log{App: app, Message: fmt.Sprintf("report day %s unreadable error=%v", d.Format(time.DateOnly), err)})
			continue
		}
		for user, t := range totals {
			out[user] = out[user].plus(t)
		}
	}
	return out
}

func line(label string, t Totals) string {
	pot := 0.0
	if t.Turnover > 0 {
		pot = t.Profit / t.Turnover * 100
	}
	s := fmt.Sprintf("%s: turnover $%.2f · profit %s · POT %.1f%% · %d/%d accepted", label, t.Turnover, signed(t.Profit), pot, t.Accepted, t.Attempted)
	if t.Pending > 0 {
		s += fmt.Sprintf(" · %d pending", t.Pending)
	}
	return s
}

func signed(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("+$%.2f", v)
}
