// engine/place.go

package engine

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/logger"
)

const (
	maxOrderRef    = 32
	maxStrategyRef = 15
)

// skipped is a bet the engine chose not to send; it is not a failure.
type skipped string

func (s skipped) Error() string { return string(s) }

// Place sends o to its provider unless nothing is staked or the runner is a duplicate for this
// user. It blocks for the provider's round trip, so callers run it on its own goroutine.
func (e *Engine) Place(o Order) {
	defer logPanic()
	if !o.staked() {
		return
	}

	a := o.Account
	id := o.BetID()
	provider := o.Side.provider()
	if !a.claims.take(a, id, o.Race.MarketID, provider) {
		logger.Debug(logger.Log{App: a.App, UserID: a.UserID, ProcessID: a.ProcessID, Race: o.logRace(), Message: fmt.Sprintf("duplicate %v blocked bet_id=%s", o.Side, id)})
		return
	}

	var err error
	if o.Side == BetmaticWin {
		err = e.placeBetmatic(o, id)
	} else {
		err = e.placeBetfair(o, id)
	}
	if err == nil {
		return
	}

	a.claims.release(a, id, provider)
	l := logger.Log{App: a.App, UserID: a.UserID, ProcessID: a.ProcessID, Race: o.logRace(), Message: fmt.Sprintf("%v not placed bet_id=%s: %v", o.Side, id, err)}
	if errors.As(err, new(skipped)) {
		logger.Debug(l)
		return
	}
	logger.Error(l)
}

func (o Order) logRace() *logger.Race {
	return &logger.Race{Venue: o.Race.Venue, Number: o.Race.Number, Runner: o.Runner}
}

func (e *Engine) placeBetmatic(o Order, id string) error {
	a := o.Account
	if a.betmatic == nil {
		return errors.New("process has no betmatic session")
	}

	st := o.Stake.Betmatic
	target := st.WinStake
	if st.WinMBL {
		target = maxBetLiability(o.Race.Metro, o.Race.Code == betmatic.THOROUGHBRED)
	}

	req := betmatic.NotificationRequest{
		Type:            betmatic.FIXED_PROFIT,
		Sports:          "RACING",
		Competition:     strings.ToUpper(o.Race.Venue),
		Code:            o.Race.Code,
		EventNumber:     o.Race.Number,
		Market:          betmatic.FIXED_WIN,
		CheckMaxOdds:    true,
		CheckOdds:       true,
		MaxOdds:         float32(st.MaxOdds),
		MinOdds:         float32(st.MinOdds),
		Selection:       o.Runner,
		BookiesOverride: strings.Join(a.Bookmakers, ","),
		TargetProfit:    target * o.Unit,
		TargetBot:       a.BotID,
		Label:           strings.ToUpper(a.App),
	}

	notificationID, err := a.betmatic.PlaceBet(req)
	if err != nil {
		return err
	}

	logger.Bet(logger.BetLog{
		App: a.App, UserID: a.UserID, ProcessID: a.ProcessID, Race: o.logRace(),
		Message:  "betmatic notification created id=" + notificationID,
		BetID:    id,
		Provider: "betmatic",
		BetType:  string(req.Type),
		Target:   req.TargetProfit,
	})
	return nil
}

func (e *Engine) placeBetfair(o Order, id string) error {
	a := o.Account
	if a.betfair == nil {
		return errors.New("process has no betfair session")
	}
	admin := e.admin.Load()
	if admin == nil {
		return errors.New("no admin betfair account for prices")
	}
	if o.Race.MarketID == "" || o.SelectionID == 0 {
		return skipped("no betfair market for runner")
	}
	prices, ok := admin.Runner(o.Race.MarketID, o.SelectionID)
	if !ok {
		return skipped("no betfair prices for runner")
	}

	st := o.Stake.Betfair
	best := prices.Back[0].Price
	if o.Side == BetfairLay {
		best = prices.Lay[0].Price
	}
	switch {
	case best < 1.01:
		return skipped(fmt.Sprintf("no %v price", o.Side))
	case best < st.MinOdds || (st.MaxOdds > 0 && best > st.MaxOdds):
		return skipped(fmt.Sprintf("price %.2f outside %.2f-%.2f", best, st.MinOdds, st.MaxOdds))
	case prices.LTP < st.MinOdds:
		return skipped(fmt.Sprintf("last traded %.2f under min odds %.2f", prices.LTP, st.MinOdds))
	}

	req := betfair.BSPBetRequest{
		MarketID:    o.Race.MarketID,
		SelectionID: o.SelectionID,
		OrderRef:    id[max(0, len(id)-maxOrderRef):],
		StrategyRef: a.App[:min(len(a.App), maxStrategyRef)],
	}
	if o.Side == BetfairLay {
		// a lay's limit is the most it will lay at: 10% above the best lay, capped at max odds
		limit := CeilToBetfairTick(best * 1.1)
		if st.MaxOdds > 0 && limit > st.MaxOdds {
			limit = FloorToBetfairTick(st.MaxOdds)
		}
		if limit == 0 || limit < best {
			return skipped(fmt.Sprintf("lay limit %.2f under lay price %.2f", limit, best))
		}
		req.Side = betfair.SideLay
		req.LimitPrice = limit
		req.Liability = math.Round(st.LayStake * o.Unit)
	} else {
		// a back's limit is the least it will take: 10% below the best back, at least min odds
		limit := FloorToBetfairTick(best * 0.9)
		if limit == 0 || limit < st.MinOdds {
			return skipped(fmt.Sprintf("back limit %.2f under min odds %.2f", limit, st.MinOdds))
		}
		req.Side = betfair.SideBack
		req.LimitPrice = limit
		req.Liability = math.Max(1, math.Round(st.BackStake*o.Unit/(best-1)*100)/100)
	}

	betID, err := a.betfair.PlaceBet(req)
	if err != nil {
		return err
	}

	logger.Bet(logger.BetLog{
		App: a.App, UserID: a.UserID, ProcessID: a.ProcessID, Race: o.logRace(),
		Message:  fmt.Sprintf("betfair bsp accepted bet=%s limit=%.2f ltp=%.2f", betID, req.LimitPrice, prices.LTP),
		BetID:    id,
		Provider: "betfair",
		BetType:  req.Side,
		Stake:    req.Liability,
		Odds:     best,
	})
	return nil
}
