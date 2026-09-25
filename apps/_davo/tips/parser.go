// packages/betting/parser.go

package betting

import (
	"fmt"
	logger "pegasus_suite/logger"
	"regexp"
	"strconv"
	"strings"
)

var (
	raceNumberRegex       = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])R(\d+)(?:[^A-Za-z0-9]|$)`)
	runnerNumberNameRegex = regexp.MustCompile(`(?i)(?:#|no\.?)\s*(\d+)[ \t]+[*\W_]*([A-Z][A-Z 'À-ÖØ-öø-ÿ-]+)[*\W_]*`)
	ratedOddsRegex        = regexp.MustCompile(`(?i)rated\W*\$?(\d+(?:\.\d+)?)`)
	betStakeRegex         = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*units?`)
	looseRaceNumberRegex  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:race\s*|r\s*)(\d{1,2})(?:[^A-Za-z0-9]|$)`)

	// anything that is likely to represent a bet
	betSignals = regexp.MustCompile(`(?i)(units?\b|\bu\b|\bwin\b|\bplace\b|\brated\b|\bodds\b|\bback\b|\blay\b|\bbet\b|each\s*way|\be/?w\b|\$\s*\d|\d+\s*u\b|(^|\W)R\s*\d|#\s*\d)`)
)

func LooksLikeBet(text string) bool {
	hit := betSignals.FindString(text)
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("bet signal check matched=%v signal=%v text=%v", hit != "", hit, text),
	})
	return hit != ""
}

func SniffRaceNumber(text string) int {
	m := looseRaceNumberRegex.FindStringSubmatch(text)
	if m == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("race number sniff: no match text=%v", text),
		})
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("race number sniff: unparseable capture=%v error=%v", m[1], err),
		})
		return 0
	}
	logger.Debug(logger.InfoLog{
		Message:     fmt.Sprintf("race number sniff: hit matched=%v", m[0]),
		RaceDetails: &logger.Race{Number: n},
	})
	return n
}

func Parse(text string) (DavoBet, error) {
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: start text=%v", text),
	})

	raceNumberMatch := raceNumberRegex.FindStringSubmatch(text)
	if raceNumberMatch == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("strict parse: no race number text=%v", text),
		})
		return DavoBet{}, fmt.Errorf("no race match")
	}
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: race number capture=%v", raceNumberMatch[1]),
	})

	runnerNumberNameMatch := runnerNumberNameRegex.FindStringSubmatch(text)
	if runnerNumberNameMatch == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("strict parse: no runner number/name text=%v", text),
		})
		return DavoBet{}, fmt.Errorf("no runner number and/or name match")
	}
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: runner number=%v name=%v", runnerNumberNameMatch[1], runnerNumberNameMatch[2]),
	})

	stakeMatch := betStakeRegex.FindStringSubmatch(text)
	if stakeMatch == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("strict parse: no stake text=%v", text),
		})
		return DavoBet{}, fmt.Errorf("no stake match")
	}
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: stake capture=%v", stakeMatch[1]),
	})

	ratedOddsMatch := ratedOddsRegex.FindStringSubmatch(text)
	if ratedOddsMatch == nil {
		logger.Debug(logger.InfoLog{
			Message: fmt.Sprintf("strict parse: no rated odds text=%v", text),
		})
		return DavoBet{}, fmt.Errorf("no rated odds match")
	}
	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: rated odds capture=%v", ratedOddsMatch[1]),
	})

	raceNumber, err := strconv.ParseInt(raceNumberMatch[1], 10, 64)
	if err != nil {
		return DavoBet{}, fmt.Errorf("unable to parse race number to int")
	}

	runnerNumber, err := strconv.ParseInt(runnerNumberNameMatch[1], 10, 32)
	if err != nil {
		return DavoBet{}, fmt.Errorf("unable to parse runner number to float")
	}

	runnerName := strings.TrimSpace(runnerNumberNameMatch[2])

	stake, err := strconv.ParseFloat(stakeMatch[1], 64)
	if err != nil {
		return DavoBet{}, fmt.Errorf("unable to parse stake to float")
	}

	ratedOdds, err := strconv.ParseFloat(ratedOddsMatch[1], 64)
	if err != nil {
		return DavoBet{}, fmt.Errorf("unable to parse rated odds to float")
	}

	market := "WIN"
	if strings.Contains(strings.ToLower(text), "place") {
		market = "PLACE"
	}

	bet := DavoBet{
		RaceNumber: int(raceNumber),
		RunnerNo:   int(runnerNumber),
		RunnerName: runnerName,
		Stake:      stake,
		Market:     market,
		RatedOdds:  ratedOdds,
	}

	logger.Debug(logger.InfoLog{
		Message: fmt.Sprintf("strict parse: ok selection=%v", bet.String()),
	})
	return bet, nil
}
