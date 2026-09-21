// betmatic/bet.go

package betmatic

import (
	"fmt"
	"strings"

	"pegasus_suite/betting"
)

// Statuses of the per-bookmaker bets under a notification. These are
// unconfirmed until checked against a real resulted notification: a status
// not listed here keeps the bet pending, so a wrong guess delays a result
// rather than inventing one.
var (
	statusSettled = map[string]bool{"WON": true, "WIN": true, "LOST": true, "LOSE": true, "LOSS": true, "SETTLED": true, "RESULTED": true}
	statusVoid    = map[string]bool{"VOID": true, "VOIDED": true, "REFUNDED": true, "CANCELLED": true, "CANCELED": true, "SCRATCHED": true}
	statusFailed  = map[string]bool{"FAILED": true, "REJECTED": true, "ERROR": true}
)

// GetBet results one notification from its per-bookmaker bets
// (GET /bet/notification/{id}/). It is resulted once every bookmaker bet is
// final; bookmakers that refused it are left out of the totals.
func (bc *Client) GetBet(id string) (betting.Bet, error) {
	resp, err := bc.GetNotificationBets(id)
	if err != nil {
		return betting.Bet{}, fmt.Errorf("betmatic notification %s: %w", id, err)
	}
	b := foldNotificationBets(resp.Bets)
	b.ID = id
	b.Provider = betting.ProviderBetmatic
	return b, nil
}

func foldNotificationBets(bets []NotificationBet) betting.Bet {
	b := betting.Bet{Status: betting.BetPending}
	if len(bets) == 0 {
		return b
	}

	var weighted float64
	accepted := false
	for _, x := range bets {
		st := strings.ToUpper(strings.TrimSpace(x.Status))
		switch {
		case statusFailed[st] || x.SubmitError != "":
			continue
		case statusSettled[st] || statusVoid[st]:
			accepted = true
			b.Stake += float64(x.Amount)
			b.Profit += float64(x.Profit)
			weighted += float64(x.Amount) * float64(x.CurrentOdds)
		default:
			return betting.Bet{Status: betting.BetPending}
		}
	}

	switch {
	case !accepted:
		b.Status = betting.BetLapsed
	case b.Profit > 0:
		b.Status = betting.BetWon
	case b.Profit < 0:
		b.Status = betting.BetLost
	default:
		b.Status = betting.BetVoid
	}
	if b.Stake > 0 {
		b.Odds = weighted / b.Stake
	}
	return b
}
