// engine/telegram.go

package engine

import (
	"context"
	"fmt"
	"pegasus_suite/apps/davo/anthropic"
	"pegasus_suite/apps/davo/core"
	"pegasus_suite/apps/davo/telegram"
	"pegasus_suite/apps/davo/tips"
	"strings"
	"time"

	logger "pegasus_suite/logger"
)

const (
	// ocrTimeout bounds a single photo read. onScrape runs on the polling loop,
	// so this is also the longest a photo tip can hold up the next message.
	ocrTimeout = 30 * time.Second

	// extractTimeout bounds the fallback. Generous because the whole book can be
	// in the prompt, and a slow answer still beats no bet.
	extractTimeout = 45 * time.Second
)

func (e *Engine) StartPolling(ctx context.Context) {
	if e.telegram == nil {
		logger.Warn(logger.ErrorLog{
			Message: "telegram client not configured; engine will not poll for selections",
		})
		return
	}

	go func() {
		if err := e.telegram.Poll(ctx, e.onScrape); err != nil && ctx.Err() == nil {
			logger.Error(logger.ErrorLog{
				Message: fmt.Sprintf("telegram polling stopped unexpectedly error=%v", err),
			})
		}
	}()
}

func (e *Engine) onScrape(ctx context.Context, msg telegram.Message) {
	text := strings.TrimSpace(msg.Text)

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("scrape: message received text=%v text_len=%v has_photo=%v", text, len(text), msg.PhotoFileID != ""),
	})

	if strings.HasPrefix(text, "/") {
		logger.Debug(logger.InfoLog{
			Message: "scrape: slash command, ignoring",
		})
		return
	}

	// A photo may be carrying the whole tip. Read it first so everything below
	// works on the same thing: text.
	if msg.PhotoFileID != "" {
		// Arm before reading. The read costs a second or two, arming is
		// idempotent, so the bookies are ready by the time we know what this is.
		logger.Debug(logger.InfoLog{Message: "arming bookies, reading tip photo"})
		e.processPreBetMessage()

		transcript, err := e.readPhoto(ctx, msg)
		if err != nil {
			logger.Warn(logger.ErrorLog{
				Message: fmt.Sprintf("unable to read tip photo error=%v", err),
			})
		} else {
			logger.Debug(logger.InfoLog{
				Message: fmt.Sprintf("scrape: transcript replaces message text was=%v now=%v", text, transcript),
			})
			text = transcript
		}
	}

	if text == "" {
		logger.Debug(logger.InfoLog{
			Message: "scrape: nothing to interpret, dropping",
		})
		return
	}

	bet, match, err := e.interpret(ctx, text)
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("scrape: interpret produced no bet error=%v", err),
		})
		// Not a bet, or we could not make one out of it. Arm and move on.
		logger.Warn(logger.ErrorLog{
			Message: fmt.Sprintf("arming bookies for unparseable (warning) message error=%v text=%v", err, text),
		})
		e.processPreBetMessage()
		return
	}

	// Backstop. Whether the bet came from the regex or the model, we do not bet
	// without a rated price.
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("scrape: rated odds backstop rated_odds=%v passes=%v", bet.RatedOdds, bet.RatedOdds > 0),
	})

	if bet.RatedOdds <= 0 {
		logger.Warn(logger.ErrorLog{
			Message: fmt.Sprintf("skipping selection with no rated odds selection=%v", bet.String()),
		})
		return
	}

	msgOut := core.DavoRaceMessage{
		Venue:        match.Venue,
		RaceNumber:   bet.RaceNumber,
		RunnerNumber: match.RunnerNo,
		RunnerName:   match.RunnerName,
		UnitSize:     bet.Stake,
		Market:       bet.Market,
		RatedOdds:    bet.RatedOdds,
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("fanning out selection units=%v market=%v rated_odds=%v name_match=%.0f%%", msgOut.UnitSize, msgOut.Market, msgOut.RatedOdds, match.Score*100),
		RaceDetails: &logger.Race{
			Venue:      msgOut.Venue,
			Number:     msgOut.RaceNumber,
			Runner:     msgOut.RunnerNumber,
			RunnerName: msgOut.RunnerName,
		},
	})
	logger.Info(logger.InfoLog{Message: "Mr Sean Combs approves of the above bet."})

	e.processbetMessage(msgOut)
}

// interpret turns a message into a bet. The strict regex plus local matching is
// tried first and answers in microseconds; anything it cannot handle that still
// looks remotely like a bet goes to the model.
func (e *Engine) interpret(ctx context.Context, text string) (betting.DavoBet, betting.EventMatch, error) {
	eventMap := e.adminClient.GetEventMap()

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("interpret: start event_map_races=%v", len(eventMap)),
	})

	bet, parseErr := betting.Parse(text)
	if parseErr == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("interpret: strict parse hit, trying local resolve selection=%v", bet.String()),
		})

		match, err := betting.ResolveEvent(eventMap, bet.RaceNumber, bet.RunnerNo, bet.RunnerName)
		if err == nil {
			logger.Debug(logger.InfoLog{
				Message:     fmt.Sprintf("interpret: resolved locally, no model call runner=%v", match.RunnerName),
				RaceDetails: &logger.Race{Venue: match.Venue},
			})
			return bet, match, nil
		}
		// Parsed but unresolved: the model gets the field for that race.
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("local resolve failed, escalating selection=%v error=%v", bet.String(), err),
		})
		return e.extract(ctx, text, bet.RaceNumber)
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("interpret: strict parse missed error=%v", parseErr),
	})

	if !betting.LooksLikeBet(text) {
		logger.Debug(logger.InfoLog{
			Message: "interpret: no bet signals, not escalating",
		})
		return betting.DavoBet{}, betting.EventMatch{}, fmt.Errorf("no bet signals in message: %w", parseErr)
	}

	// Did not parse but reads like a bet. Narrow the field by race number if we
	// can sniff one out; otherwise the model gets the whole book.
	sniffed := betting.SniffRaceNumber(text)
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("interpret: escalating unparsed message sniffed_race_number=%v", sniffed),
	})
	return e.extract(ctx, text, sniffed)
}

// extract hands the message and the candidate field to the model. raceNumber of
// 0 means we could not determine one, in which case every runner in every
// upcoming race is sent.
func (e *Engine) extract(ctx context.Context, text string, raceNumber int) (betting.DavoBet, betting.EventMatch, error) {
	if e.anthropic == nil {
		logger.Debug(logger.InfoLog{
			Message: "extract: no anthropic client, cannot escalate",
		})
		return betting.DavoBet{}, betting.EventMatch{}, fmt.Errorf("anthropic client not configured")
	}

	eventMap := e.adminClient.GetEventMap()

	var (
		candidates []betting.Candidate
		err        error
	)
	if raceNumber > 0 {
		logger.Debug(logger.InfoLog{
			Message:     "extract: building field for race",
			RaceDetails: &logger.Race{Number: raceNumber},
		})
		candidates, err = betting.RaceCandidates(eventMap, raceNumber)
		if err != nil {
			// The sniffed race number may simply be wrong. Fall back to the whole
			// book rather than giving up on the message.
			logger.Debug(logger.InfoLog{
				Message:     fmt.Sprintf("no field for race number, sending whole book error=%v", err),
				RaceDetails: &logger.Race{Number: raceNumber},
			})
			candidates, err = betting.AllCandidates(eventMap)
		}
	} else {
		logger.Debug(logger.InfoLog{
			Message: "extract: no race number, building whole book",
		})
		candidates, err = betting.AllCandidates(eventMap)
	}
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("extract: no candidates available error=%v", err),
		})
		return betting.DavoBet{}, betting.EventMatch{}, err
	}
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("extract: field built candidates=%v", len(candidates)),
	})

	if ok, reason := e.gate.admit(tipKey(text)); !ok {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("extract: refused by gate reason=%v", reason),
		})
		return betting.DavoBet{}, betting.EventMatch{}, fmt.Errorf("%s", reason)
	}

	sent := make([]anthropic.Candidate, 0, len(candidates))
	for _, c := range candidates {
		sent = append(sent, anthropic.Candidate{
			RaceNumber: c.RaceNumber,
			Venue:      c.Venue,
			RunnerName: c.RunnerName,
			RunnerNo:   c.RunnerNo,
		})
	}

	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("escalating message to model candidates=%v text=%v", len(sent), text),
		RaceDetails: &logger.Race{Number: raceNumber},
	})

	extractCtx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("extract: calling model candidates=%v timeout=%v", len(sent), extractTimeout),
	})

	out, err := e.anthropic.ExtractBet(extractCtx, text, sent)
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("extract: model call failed error=%v", err),
		})
		return betting.DavoBet{}, betting.EventMatch{}, err
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("extract: model replied index=%v stake_units=%v market=%v rated_odds=%v reason=%v", out.Index, out.StakeUnits, out.Market, out.RatedOdds, out.Reason),
	})

	// The model is told to decline rather than guess. A decline has to stay a
	// refusal to bet - falling back to a local best guess here would undo the
	// whole point of asking.
	if out.Index == anthropic.NotABet {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("extract: model declined reason=%v", out.Reason),
		})
		return betting.DavoBet{}, betting.EventMatch{}, fmt.Errorf("not a bet: %s", out.Reason)
	}

	chosen := candidates[out.Index]

	// Everything identifying the runner comes from our own event data; the model
	// only supplies what the message knows.
	bet := betting.DavoBet{
		RaceNumber: chosen.RaceNumber,
		RunnerNo:   chosen.RunnerNo,
		RunnerName: chosen.RunnerName,
		Venue:      chosen.Venue,
		Stake:      out.StakeUnits,
		Market:     out.Market,
		RatedOdds:  out.RatedOdds,
	}

	match := betting.EventMatch{
		Venue:      chosen.Venue,
		RunnerName: chosen.RunnerName,
		RunnerNo:   chosen.RunnerNo,
		Score:      chosen.Score,
	}

	logger.Info(logger.InfoLog{
		Message: fmt.Sprintf("model extracted bet selection=%v reason=%v", bet.String(), out.Reason),
	})

	return bet, match, nil
}

// readPhoto downloads a channel photo and transcribes it.
func (e *Engine) readPhoto(ctx context.Context, msg telegram.Message) (string, error) {
	if e.anthropic == nil {
		return "", fmt.Errorf("photo tip received but anthropic client not configured")
	}

	if ok, reason := photoUsable(msg); !ok {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("photo: rejected before download reason=%v", reason),
		})
		return "", fmt.Errorf("photo skipped: %s", reason)
	}

	if ok, reason := e.gate.admit(photoKey(msg.PhotoFileID)); !ok {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("photo: refused by gate reason=%v", reason),
		})
		return "", fmt.Errorf("photo skipped: %s", reason)
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("photo: downloading file_id=%v", msg.PhotoFileID),
	})

	image, err := e.telegram.DownloadFile(ctx, msg.PhotoFileID)
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("photo: download failed error=%v", err),
		})
		return "", fmt.Errorf("downloading photo: %w", err)
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("photo: downloaded, sending to OCR bytes=%v timeout=%v", len(image), ocrTimeout),
	})

	ocrCtx, cancel := context.WithTimeout(ctx, ocrTimeout)
	defer cancel()

	text, err := e.anthropic.ExtractText(ocrCtx, image)
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("photo: OCR failed error=%v", err),
		})
		return "", fmt.Errorf("reading photo: %w", err)
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("photo: OCR returned chars=%v", len(text)),
	})

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("read tip from photo text=%v", text),
	})

	return text, nil
}
