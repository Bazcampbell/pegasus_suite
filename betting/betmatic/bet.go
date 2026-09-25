// betmatic/bet.go

package betmatic

import (
	"fmt"
	"strconv"

	"pegasus_suite/betting"
	"pegasus_suite/platform/util"
)

const notificationPageSize = 100

// MeetingBets returns one bet per notification on the account for a meeting date (YYYY-MM-DD).
func (bc *Client) MeetingBets(date string) ([]betting.Bet, error) {
	var out []betting.Bet
	for page := 1; ; page++ {
		resp, err := bc.GetNotifications(GetNotificationsRequest{
			MeetingDateFrom: date,
			MeetingDateTo:   date,
			Page:            util.FlexInt(page),
			PageSize:        notificationPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("betmatic notifications %s page %d: %w", date, page, err)
		}
		for _, r := range resp.Results {
			out = append(out, r.bet())
		}
		if resp.Next == "" || len(resp.Results) == 0 {
			return out, nil
		}
	}
}

func (r Result) bet() betting.Bet {
	b := betting.Bet{ID: strconv.FormatInt(r.ID, 10), Provider: betting.ProviderBetmatic, Bot: r.TargetBot}
	accepted := float64(r.TotalAccepted)
	switch {
	case accepted == 0 && (r.IsCanceled || r.Resulted()):
		b.Status = betting.BetLapsed
	case !r.Resulted():
		b.Status = betting.BetPending
	case r.Profit > 0:
		b.Status, b.Liability, b.Profit = betting.BetWon, accepted, float64(r.Profit)
	case r.Profit < 0:
		b.Status, b.Liability, b.Profit = betting.BetLost, accepted, float64(r.Profit)
	default:
		b.Status = betting.BetVoid
	}
	return b
}
