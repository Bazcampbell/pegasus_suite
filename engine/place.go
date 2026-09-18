// engine/place.go
//
// Order → provider request → bookmaker. Runs on the caller's goroutine; the
// process spawns one per bet so no bookmaker holds up the next message.
//
// Nothing here looks anything up: the account, the venue names, the live
// Betfair book and the stake all arrive on the Order. The only work is
// arithmetic and one network call.

package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"pegasus_suite/betting/betfair"
	"pegasus_suite/betting/betmatic"
	"pegasus_suite/logger"
)

const maxBetfairCustomerRef = 32

func (e *Engine) Place(o Order) {
	switch o.Side {
	case BetmaticWin:
		if o.Stake.BetsBetmatic() {
			e.placeBetmatic(o)
		}
	case BetfairBack:
		if o.Stake.Betfair.BackStake > 0 {
			e.placeBetfair(o)
		}
	case BetfairLay:
		if o.Stake.Betfair.LayStake > 0 {
			e.placeBetfair(o)
		}
	}
}

func (o Order) race() *logger.RaceDetails {
	return &logger.RaceDetails{Venue: o.Event.VenueName, RaceNumber: o.Event.RaceNumber, RunnerNumber: o.Runner}
}

func (e *Engine) placeBetmatic(o Order) {
	a := o.Account
	if !a.HasBetmatic() {
		logger.Error(logger.ErrorLog{
			Message: "scope stakes betmatic but the process has no betmatic session", UserID: a.UserID, ProcessID: a.ProcessID, RaceDetails: o.race(),
		})
		return
	}

	venue := o.Event.Betmatic
	if venue == nil {
		logger.Error(logger.ErrorLog{
			Message: fmt.Sprintf("no betmatic venue for %s; cannot bet", o.Event.VenueName), UserID: a.UserID, ProcessID: a.ProcessID, RaceDetails: o.race(),
		})
		return
	}

	// MBL bets the bookmaker maximum rather than targeting a liability, and that
	// maximum depends on the track. GetLiability owns the table.
	st := o.Stake.Betmatic
	lia := st.WinStake
	if st.WinMBL {
		lia, _ = GetLiability(venue.IsMetro, true, true, false, 0, 0)
	}

	n := betmatic.NotificationRequest{
		Type:            betmatic.FIXED_PROFIT,
		Sports:          "RACING",
		Competition:     strings.ToUpper(venue.Name),
		Code:            o.Event.Code,
		EventNumber:     o.Event.RaceNumber,
		Market:          betmatic.FIXED_WIN,
		CheckMaxOdds:    true,
		CheckOdds:       true,
		MaxOdds:         float32(st.MaxOdds),
		MinOdds:         float32(st.MinOdds),
		Selection:       o.Runner,
		BookiesOverride: strings.Join(a.Bookmakers, ","),
		TargetProfit:    lia * o.Unit,
		TargetBot:       a.BotID,
		Label:           o.Label,
	}

	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("placing betmatic bet venue=%v runner=%v target_profit=%.2f (lia %.2f x unit %.2f) mbl=%v odds=%.2f-%.2f", n.Competition, o.Runner, n.TargetProfit, lia, o.Unit, st.WinMBL, st.MinOdds, st.MaxOdds),
		UserID:      a.UserID,
		ProcessID:   a.ProcessID,
		RaceDetails: o.race(),
	})

	if err := a.betmatic.PlaceBet(n); err != nil {
		logger.Error(logger.ErrorLog{
			Message: fmt.Sprintf("betmatic bet rejected error=%v", err), UserID: a.UserID, ProcessID: a.ProcessID, Request: n, RaceDetails: o.race(),
		})
		return
	}

	// Betmatic sizes and fills the notification itself, so this is a request,
	// not a bet: the BET log comes from Results once it settles.
	logger.Info(logger.InfoLog{
		Message:     fmt.Sprintf("betmatic bet requested %s R%d runner %d %s target $%.2f", n.Competition, n.EventNumber, n.Selection, n.Type, n.TargetProfit),
		UserID:      a.UserID,
		ProcessID:   a.ProcessID,
		RaceDetails: o.race(),
	})

	if a.bmClient != nil {
		e.results.Watch(a.bmClient, o.Label, a.ProcessID)
	}
}

func (e *Engine) placeBetfair(o Order) {
	a := o.Account
	if !a.HasBetfair() {
		logger.Error(logger.ErrorLog{
			Message: "scope stakes betfair but the process has no betfair session", UserID: a.UserID, ProcessID: a.ProcessID, RaceDetails: o.race(),
		})
		return
	}

	race := o.Event.Betfair
	if race == nil {
		logger.Warn(logger.ErrorLog{Message: "unable to resolve betfair race", RaceDetails: o.race()})
		return
	}

	runner, ok := race.Runners[o.Runner]
	if !ok {
		logger.Warn(logger.ErrorLog{Message: "unable to get betfair runner from race", Response: race.Runners, RaceDetails: o.race()})
		return
	}

	selectionID, err := strconv.ParseInt(runner.SelectionID, 10, 64)
	if err != nil {
		logger.Error(logger.ErrorLog{
			Message: fmt.Sprintf("unusable betfair selection id error=%v", err), UserID: a.UserID, ProcessID: a.ProcessID, Request: runner, RaceDetails: o.race(),
		})
		return
	}

	st := o.Stake.Betfair

	// updateRunners writes the back book best-first and sorts the lay book
	// ascending, so [0] is the best available price on either side.
	book := runner.Back
	if o.Side == BetfairLay {
		book = runner.Lay
	}

	if len(book) == 0 || book[0].Price < 1.01 {
		logger.Warn(logger.ErrorLog{Message: fmt.Sprintf("no %v price for betfair runner", o.Side), Request: runner, RaceDetails: o.race()})
		return
	}

	price := book[0].Price

	if price < st.MinOdds || (st.MaxOdds > 0 && price > st.MaxOdds) {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("betfair %v price %.2f outside %.2f-%.2f, skipping", o.Side, price, st.MinOdds, st.MaxOdds), UserID: a.UserID, ProcessID: a.ProcessID, RaceDetails: o.race(),
		})
		return
	}

	if runner.LTP < st.MinOdds {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("betfair last traded %.2f under min odds %.2f, skipping", runner.LTP, st.MinOdds), UserID: a.UserID, ProcessID: a.ProcessID, RaceDetails: o.race(),
		})
		return
	}

	req := betfair.BSPBetRequest{
		MarketID:    runner.MarketID,
		SelectionID: selectionID,
		CustomerRef: o.Label,
	}
	if len(req.CustomerRef) > maxBetfairCustomerRef {
		req.CustomerRef = req.CustomerRef[:maxBetfairCustomerRef]
	}

	if o.Side == BetfairLay {
		limit := CeilToBetfairTick(price * 1.1)
		if st.MaxOdds > 0 && limit > st.MaxOdds {
			limit = FloorToBetfairTick(st.MaxOdds)
		}
		if limit == 0 || limit < price {
			logger.Debug(logger.InfoLog{Message: fmt.Sprintf("betfair lay limit %.2f under lay price %.2f, skipping", limit, price), RaceDetails: o.race()})
			return
		}

		req.Side = betfair.SideLay
		req.LimitPrice = limit
		req.Liability = math.Round(st.LayStake * o.Unit)
	} else {
		limit := FloorToBetfairTick(price * 0.9)
		if limit == 0 || limit < st.MinOdds {
			logger.Debug(logger.InfoLog{Message: fmt.Sprintf("betfair limit %.2f under min odds %.2f, skipping", limit, st.MinOdds), RaceDetails: o.race()})
			return
		}

		req.Side = betfair.SideBack
		req.LimitPrice = limit
		req.Liability = math.Max(1, math.Round(st.BackStake*o.Unit/(price-1)*100)/100)
	}

	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("placing betfair %v runner=%v market=%v best=%.2f limit=%.2f ltp=%.2f size=%.2f", o.Side, o.Runner, req.MarketID, price, req.LimitPrice, runner.LTP, req.Liability),
		UserID:      a.UserID,
		ProcessID:   a.ProcessID,
		RaceDetails: o.race(),
	})

	if err := a.betfair.PlaceBet(req); err != nil {
		logger.Error(logger.ErrorLog{
			Message: fmt.Sprintf("betfair %v rejected error=%v", o.Side, err), UserID: a.UserID, ProcessID: a.ProcessID, Request: req, RaceDetails: o.race(),
		})
		return
	}

	// An INFO, not a BET: the order sits PENDING until the market turns in-play
	// and the SP is struck, so neither the price nor the stake actually on is
	// knowable here.
	logger.Info(logger.InfoLog{
		Message:     fmt.Sprintf("betfair bsp bet accepted market=%s selection=%d %s liability $%.2f ref=%s", req.MarketID, req.SelectionID, strings.ToUpper(req.Side), req.Liability, req.CustomerRef),
		UserID:      a.UserID,
		ProcessID:   a.ProcessID,
		RaceDetails: o.race(),
	})
}
