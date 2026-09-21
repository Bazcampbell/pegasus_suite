package telegram

import (
	"strings"
	"testing"

	"pegasus_suite/logger"
)

var race = &logger.RaceDetails{Venue: "ROCKHAMPTON", RaceNumber: 7, RunnerNumber: 1, RunnerName: "Brank Daddy"}

func TestLogCardIsTheSameForEveryLevel(t *testing.T) {
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		card := formatTelegram(logger.Record{
			Level: level, Application: "pegasus", Message: "unable to resolve betfair race",
			Username: "punter", ProcessID: "bot-a", Race: race,
		})
		for _, line := range []string{
			"<b>" + level + "</b> · pegasus",
			"unable to resolve betfair race",
			"🏇 <b>ROCKHAMPTON R7</b> · 🐎 <b>#1</b> Brank Daddy",
			"👤 punter · ⚙️ bot-a",
		} {
			if !strings.Contains(card, line) {
				t.Fatalf("%s card missing %q:\n%s", level, line, card)
			}
		}
	}
}

func TestLogCardWithoutARace(t *testing.T) {
	card := formatTelegram(logger.Record{Level: "ERROR", Application: "pegasus", Message: "betfair keepAlive failed"})
	if card != "🔴 <b>ERROR</b> · pegasus\n\nbetfair keepAlive failed" {
		t.Fatalf("card:\n%s", card)
	}
}

func TestBetCardShowsTheMoney(t *testing.T) {
	card := formatTelegram(logger.Record{
		Level: "BET", Application: "pegasus", Message: "betmatic bet resulted ROCKHAMPTON R7 runner 1",
		Username: "punter", ProcessID: "bot-a", Race: race,
		Bet: &logger.BetLog{
			Endpoint: "BETMATIC", BetType: "Fixed Profit", Market: "Fixed Win", Ref: "pegasus_a1b2c3",
			Requested: 10, Accepted: 8, Odds: 4.6, TargetLia: 240, Profit: 28.8, Result: "WIN",
		},
	})
	for _, line := range []string{
		"🟢 <b>BET</b> · pegasus",
		"🏇 <b>ROCKHAMPTON R7</b> · Fixed Win",
		"🐎 Runner <b>#1</b> Brank Daddy",
		"💵 Requested $10.00 · Accepted $8.00 @ 4.60",
		"🎯 Target $240.00",
		"🏁 <b>WIN</b> · 💰 +$28.80",
		"🔌 BETMATIC · Fixed Profit · pegasus_a1b2c3",
		"👤 punter · ⚙️ bot-a",
	} {
		if !strings.Contains(card, line) {
			t.Fatalf("bet card missing %q:\n%s", line, card)
		}
	}
}

func TestCardsEscapeHTML(t *testing.T) {
	card := formatTelegram(logger.Record{Level: "ERROR", Message: "bad <price> & stuff", Race: &logger.RaceDetails{Venue: "A&B"}})
	if !strings.Contains(card, "bad &lt;price&gt; &amp; stuff") || !strings.Contains(card, "A&amp;B") {
		t.Fatalf("not escaped:\n%s", card)
	}
}
