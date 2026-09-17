// packages/tenant/betting.go

package tenant

import (
	"context"
	"racing_wagering/apps/davo/core"
	"fmt"
	"log/slog"
	"strings"

	"racing_wagering/betting/betmatic"
	betengine "github.com/Bazcampbell/bazbet-sdk/engine"
	logger "racing_wagering/logger"
)

// run is the per-Start goroutine. It owns the ctx passed to it (which is a
// snapshot of p.ctx at Start time), so subsequent Stop/Start cycles can
// safely reassign p.ctx without affecting the still-draining goroutine.
// Must keep draining RaceDataInbox so the engine's fan-out never blocks.
func (p *Process) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.InfoLog{
				Message:   fmt.Sprintf("process stopped reason=%v", ctx.Err()),
				UserID:    p.Settings.UserID,
				ProcessID: p.Settings.ID,
			})
			return
		case raceData := <-p.RaceDataInbox:
			p.handleBetmaticSelection(raceData)
		}
	}
}

func (p *Process) handleBetmaticSelection(message core.DavoRaceMessage) {
	minOdds := message.RatedOdds / (1 + (p.Settings.MinOddsThreshold / 100))

	var meta *[]string // used in bet labels
	var stake float64

	if p.Settings.IsTest {
		stake = 1
		meta = &[]string{"test"}
	} else {
		stake = (message.UnitSize * p.Settings.TargetLiability) / (minOdds - 1)
		meta = nil
	}

	slog.Debug("handling betmatic selection", "message", message, "request", message, "response", map[string]any{"minOdds": minOdds, "stake": stake})

	market := betmatic.FIXED_WIN
	if message.Market == "place" {
		market = betmatic.FIXED_PLACE
	}

	notification := betmatic.NotificationRequest{
		Type:              betmatic.HIGH_ODDS_FIRST,
		Sports:            "RACING",
		Competition:       strings.ToUpper(message.Venue),
		Code:              betmatic.THOROUGHBRED,
		EventNumber:       message.RaceNumber,
		Market:            market,
		Selection:         message.RunnerNumber,
		Stake:             stake,
		TotalWager:        stake,
		AllowedDoubleBets: true,
		EnsureTotalWager:  true,
		TargetBetType:     "CASH",
		CheckOdds:         true,
		MinOdds:           float32(minOdds),
		BookiesOverride:   strings.Join(p.Settings.Bookmakers, ","),
		ChooseRandomBots:  false,
		TargetBot:         p.Settings.BotID,
	}

	race := &logger.RaceDetails{
		Venue:        message.Venue,
		RaceNumber:   message.RaceNumber,
		RunnerNumber: message.RunnerNumber,
		RunnerName:   message.RunnerName,
	}

	if err := betengine.PlaceBetmaticBet(p.Settings.BetmaticEmail, p.Settings.ID, notification, meta); err != nil {
		logger.Error(logger.ErrorLog{
			Message:     fmt.Sprintf("unable to send bet notification error=%v", err),
			UserID:      p.Settings.UserID,
			ProcessID:   p.Settings.ID,
			Request:     notification,
			RaceDetails: race,
		})
	}
}
