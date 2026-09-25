// davo/reader.go
//
// Telegram post → tip. The strict parser and local runner matching answer in
// microseconds; anything they cannot place that still reads like a bet goes to
// the model with the real field. The runner always comes from Betmatic's own
// event data, never from the model.

package davo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"pegasus_suite/apps/davo/anthropic"
	"pegasus_suite/apps/davo/process"
	"pegasus_suite/apps/davo/telegram"
	"pegasus_suite/apps/davo/tips"
	"pegasus_suite/logger"
)

const (
	// onPost runs on the polling loop, so these also bound how long one post holds up the next
	ocrTimeout     = 30 * time.Second
	extractTimeout = 45 * time.Second
)

// onPost turns a channel post into a tip and offers it to every process; anything else arms the bookies.
func (a *App) onPost(ctx context.Context, msg telegram.Message) {
	defer logger.Recover(Name)
	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/") {
		return
	}

	if msg.PhotoFileID != "" {
		// arming is idempotent and the read takes seconds, so arm first
		a.arm()
		transcript, err := a.readPhoto(ctx, msg)
		if err != nil {
			logger.Warn(logger.Log{App: Name, Message: fmt.Sprintf("unable to read tip photo error=%v", err)})
		} else {
			text = transcript
		}
	}
	if text == "" {
		return
	}

	bet, match, err := a.interpret(ctx, text)
	if err != nil {
		logger.Info(logger.Log{App: Name, Message: fmt.Sprintf("post is not a bet, arming bookies error=%v text=%q", err, text)})
		a.arm()
		return
	}
	if bet.RatedOdds <= 0 {
		logger.Warn(logger.Log{App: Name, Message: "skipping selection with no rated odds " + bet.String()})
		return
	}

	tip := process.Tip{
		Date:       match.Date,
		Venue:      match.Venue,
		RaceNumber: bet.RaceNumber,
		Runner:     match.RunnerNo,
		RunnerName: match.RunnerName,
		Units:      bet.Stake,
		Market:     bet.Market,
		RatedOdds:  bet.RatedOdds,
	}
	logger.Info(logger.Log{
		App:     Name,
		Race:    &logger.Race{Venue: tip.Venue, Number: tip.RaceNumber, Runner: tip.Runner, RunnerName: tip.RunnerName},
		Message: fmt.Sprintf("tip units=%v market=%v rated_odds=%v name_match=%.0f%%", tip.Units, tip.Market, tip.RatedOdds, match.Score*100),
	})

	for _, p := range a.processes() {
		p.Offer(tip)
	}
}

func (a *App) arm() {
	for _, p := range a.processes() {
		p.Arm()
	}
}

// interpret returns the bet a message describes, trying the strict parser before the model.
func (a *App) interpret(ctx context.Context, text string) (tips.DavoBet, tips.EventMatch, error) {
	events := a.events.GetEventMap()

	bet, parseErr := tips.Parse(text)
	if parseErr == nil {
		match, err := tips.ResolveEvent(events, bet.RaceNumber, bet.RunnerNo, bet.RunnerName)
		if err == nil {
			return bet, match, nil
		}
		logger.Debug(logger.Log{App: Name, Message: fmt.Sprintf("local resolve failed, asking the model selection=%v error=%v", bet.String(), err)})
		return a.extract(ctx, text, bet.RaceNumber)
	}

	if !tips.LooksLikeBet(text) {
		return tips.DavoBet{}, tips.EventMatch{}, fmt.Errorf("no bet signals in message: %w", parseErr)
	}
	return a.extract(ctx, text, tips.SniffRaceNumber(text))
}

// extract asks the model to pick the runner from the field for raceNumber, or the whole book when 0.
func (a *App) extract(ctx context.Context, text string, raceNumber int) (tips.DavoBet, tips.EventMatch, error) {
	if a.model == nil {
		return tips.DavoBet{}, tips.EventMatch{}, errors.New("anthropic client not configured")
	}

	events := a.events.GetEventMap()
	candidates, err := tips.AllCandidates(events)
	if raceNumber > 0 {
		if field, fieldErr := tips.RaceCandidates(events, raceNumber); fieldErr == nil {
			candidates, err = field, nil
		}
	}
	if err != nil {
		return tips.DavoBet{}, tips.EventMatch{}, err
	}

	if ok, reason := a.gate.admit(tipKey(text)); !ok {
		return tips.DavoBet{}, tips.EventMatch{}, errors.New(reason)
	}

	sent := make([]anthropic.Candidate, len(candidates))
	for i, c := range candidates {
		sent[i] = anthropic.Candidate{RaceNumber: c.RaceNumber, Venue: c.Venue, RunnerName: c.RunnerName, RunnerNo: c.RunnerNo}
	}

	extractCtx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	out, err := a.model.ExtractBet(extractCtx, text, sent)
	if err != nil {
		return tips.DavoBet{}, tips.EventMatch{}, err
	}
	// a decline stays a decline; falling back to a local guess would defeat asking
	if out.Index == anthropic.NotABet {
		return tips.DavoBet{}, tips.EventMatch{}, fmt.Errorf("not a bet: %s", out.Reason)
	}

	chosen := candidates[out.Index]
	bet := tips.DavoBet{
		RaceNumber: chosen.RaceNumber,
		RunnerNo:   chosen.RunnerNo,
		RunnerName: chosen.RunnerName,
		Venue:      chosen.Venue,
		Stake:      out.StakeUnits,
		Market:     out.Market,
		RatedOdds:  out.RatedOdds,
	}
	logger.Info(logger.Log{App: Name, Message: fmt.Sprintf("model extracted bet selection=%v reason=%v", bet.String(), out.Reason)})
	return bet, tips.EventMatch{Date: chosen.Date, Venue: chosen.Venue, RunnerName: chosen.RunnerName, RunnerNo: chosen.RunnerNo, Score: chosen.Score}, nil
}

// readPhoto downloads a post's photo and returns the model's transcript of it.
func (a *App) readPhoto(ctx context.Context, msg telegram.Message) (string, error) {
	if a.model == nil {
		return "", errors.New("anthropic client not configured")
	}
	if ok, reason := photoUsable(msg); !ok {
		return "", errors.New(reason)
	}
	if ok, reason := a.gate.admit(photoKey(msg.PhotoFileID)); !ok {
		return "", errors.New(reason)
	}

	image, err := a.telegram.DownloadFile(ctx, msg.PhotoFileID)
	if err != nil {
		return "", fmt.Errorf("downloading photo: %w", err)
	}

	ocrCtx, cancel := context.WithTimeout(ctx, ocrTimeout)
	defer cancel()
	return a.model.ExtractText(ocrCtx, image)
}
