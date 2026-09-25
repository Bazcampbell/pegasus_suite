// davo/anthropic/extract.go
//
// The fallback for anything the strict parser could not turn into a bet. The
// message and the real field are handed to the model, which either identifies
// the runner and the bet terms or says it is not a bet at all.
//
// The model only supplies what the message knows - stake, market, rated odds.
// Venue, race number, runner name and saddlecloth number all come from our own
// event data via the chosen candidate, so a hallucinated runner is not
// representable.

package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pegasus_suite/logger"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	extractModel = "claude-opus-5"

	extractMaxTokens = 2048

	extractEffort = anthropic.OutputConfigEffortMedium
)

const NotABet = -1

const extractSystemPrompt = `You read horse racing tip messages and match them to a runner in a supplied list.

Messages arrive in inconsistent formats and are sometimes transcribed from screenshots, so they carry OCR noise. The tipster also obscures runner names on purpose: misspellings, dropped or swapped letters, phonetic spellings, lookalike characters, and digits standing in for letters. Any race number or runner number in the message is a hint, not a fact - both are frequently wrong or missing.

Decide first whether the message is actually a bet instruction. Plenty of what you receive is ordinary chatter: results, congratulations, banter, or a heads-up that a tip is coming. Those are not bets.

Answer with:
- "is_bet": true only if the message instructs a bet on a specific runner.
- "index": the number shown against your chosen candidate, or 0 if none of them is the runner referred to.
- "stake_units": the stake in units, or 0 if the message does not state one.
- "market": "WIN" or "PLACE". Use "WIN" unless the message clearly says place.
- "rated_odds": the tipster's rated or recommended price, or the price offered by a bookmaker - if the message lists several bookmaker prices and one rated price, return the rated one else return any odds you find.
- "reason": one short sentence naming the specific evidence you used.

Return is_bet false, or index 0, whenever you are not confident. A wrong answer places real money on the wrong horse, which is far worse than declining. Two candidates that are both plausible means you are not confident: return 0. Do not pick a candidate merely because it is the closest of a bad set.`

var extractSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"is_bet": map[string]any{
			"type":        "boolean",
			"description": "True only if the message instructs a bet on a specific runner.",
		},
		"index": map[string]any{
			"type":        "integer",
			"description": "The number of the chosen candidate, or 0 if none matches.",
		},
		"stake_units": map[string]any{
			"type":        "number",
			"description": "Stake in units, or 0 if not stated.",
		},
		"market": map[string]any{
			"type": "string",
			"enum": []string{"WIN", "PLACE"},
		},
		"rated_odds": map[string]any{
			"type":        "number",
			"description": "The tipster's rated price, or 0 if absent.",
		},
		"reason": map[string]any{
			"type":        "string",
			"description": "One short sentence naming the evidence used.",
		},
	},
	"required":             []string{"is_bet", "index", "stake_units", "market", "rated_odds", "reason"},
	"additionalProperties": false,
}

// Candidate is one runner the model may choose between.
type Candidate struct {
	RaceNumber int
	Venue      string
	RunnerName string
	RunnerNo   int
}

// Extraction is the model's answer. Index is a 0-based offset into the
// candidates slice passed to ExtractBet, or NotABet when it declined.
type Extraction struct {
	Index      int
	StakeUnits float64
	Market     string
	RatedOdds  float64
	Reason     string
}

type extractReply struct {
	IsBet      bool    `json:"is_bet"`
	Index      int     `json:"index"`
	StakeUnits float64 `json:"stake_units"`
	Market     string  `json:"market"`
	RatedOdds  float64 `json:"rated_odds"`
	Reason     string  `json:"reason"`
}

// ExtractBet asks the model to turn a message into a bet against the supplied
// field. Candidates should be the real runners for the race, or the whole book
// when the race number could not be determined.
func (c *Client) ExtractBet(ctx context.Context, text string, candidates []Candidate) (Extraction, error) {
	if strings.TrimSpace(text) == "" {
		return Extraction{}, fmt.Errorf("empty message")
	}
	if len(candidates) == 0 {
		return Extraction{}, fmt.Errorf("no candidates supplied")
	}

	var prompt strings.Builder
	prompt.WriteString("Message:\n")
	prompt.WriteString(text)
	prompt.WriteString("\n\nCandidates:\n")

	// Numbered from 1 so that 0 is unambiguously "none of these" and cannot be
	// confused with a valid choice.
	for i, cand := range candidates {
		fmt.Fprintf(&prompt, "%d. R%d %s - #%d %s\n", i+1, cand.RaceNumber, cand.Venue, cand.RunnerNo, cand.RunnerName)
	}

	logger.Debug(logger.Log{
		Message: fmt.Sprintf("anthropic extract: request model=%v effort=%v candidates=%v prompt_chars=%v", extractModel, extractEffort, len(candidates), prompt.Len()),
	})

	msg, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     extractModel,
		MaxTokens: extractMaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: extractSystemPrompt},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: extractEffort,
			Format: anthropic.JSONOutputFormatParam{Schema: extractSchema},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.String())),
		},
	})
	if err != nil {
		logAPIError("extract", err)
		return Extraction{}, fmt.Errorf("anthropic request: %w", err)
	}

	if msg.StopReason == anthropic.StopReasonRefusal {
		return Extraction{}, fmt.Errorf("request refused: %s", msg.StopDetails.Explanation)
	}

	logger.Debug(logger.Log{
		Message: fmt.Sprintf("anthropic extract: usage input_tokens=%v output_tokens=%v stop_reason=%v", msg.Usage.InputTokens, msg.Usage.OutputTokens, msg.StopReason),
	})

	body := responseText(msg)
	if body == "" {
		return Extraction{}, fmt.Errorf("no answer returned")
	}
	logger.Debug(logger.Log{
		Message: fmt.Sprintf("anthropic extract: raw reply json=%v", body),
	})

	var reply extractReply
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		return Extraction{}, fmt.Errorf("decoding answer %q: %w", body, err)
	}

	out := Extraction{
		Index:      NotABet,
		StakeUnits: reply.StakeUnits,
		Market:     strings.ToUpper(strings.TrimSpace(reply.Market)),
		RatedOdds:  reply.RatedOdds,
		Reason:     reply.Reason,
	}

	if out.Market != "PLACE" {
		out.Market = "WIN"
	}

	if !reply.IsBet || reply.Index == 0 {
		return out, nil
	}

	// Anything outside the list is treated as a decline rather than an error: a
	// nonsensical index is not evidence for any particular runner.
	if reply.Index < 1 || reply.Index > len(candidates) {
		out.Reason = fmt.Sprintf("model returned out-of-range index %d", reply.Index)
		return out, nil
	}

	out.Index = reply.Index - 1
	return out, nil
}
