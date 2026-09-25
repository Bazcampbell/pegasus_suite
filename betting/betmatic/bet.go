// betmatic/bet.go

package betmatic

/*
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

//func (bc *Client) GetBetRecap(id [])

/*
func foldNotificationBets(bets []NotificationBet) betting.Bet {
	b := betting.Bet{Status: betting.BetPending}
	if len(bets) == 0 {
		return b
	}

	var weighted float64
	var accepted bool
	for _, x := range bets {
		if x.Profit != 0 {
			accepted = true
			b.Stake += float64(x.Amount)
			b.Profit += float64(x.Profit)
			weighted += float64(x.Amount) * float64(x.CurrentOdds)
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
*/
