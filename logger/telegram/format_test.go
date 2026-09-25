package telegram

import (
	"strings"
	"testing"

	"pegasus_suite/logger"
)

var race = &logger.Race{Venue: "ROCKHAMPTON", Number: 7, Runner: 1, RunnerName: "Brank Daddy"}

func TestLogCardIsTheSameForEveryLevel(t *testing.T) {
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		card := formatTelegram(logger.Record{
			Level: level, Application: "pegasus", Message: "unable to resolve betfair race",
			UserID: "punter", ProcessID: "bot-a", Race: race,
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
		Level: "BET", Application: "pegasus", Message: "betfair back accepted",
		UserID: "punter", ProcessID: "bot-a", Race: race,
		Bet: &logger.BetLog{
			Provider: "betfair", BetType: "BACK", BetID: "ROCKHAMPTON:R7:1:20260925",
			Stake: 10, Odds: 4.6, Target: 240,
		},
	})
	for _, line := range []string{
		"🟢 <b>BET</b> · pegasus",
		"🏇 <b>ROCKHAMPTON R7</b>",
		"🐎 Runner <b>#1</b> Brank Daddy",
		"💵 Stake $10.00 @ 4.60 · 🎯 Target $240.00",
		"🔌 betfair · BACK · ROCKHAMPTON:R7:1:20260925",
		"👤 punter · ⚙️ bot-a",
	} {
		if !strings.Contains(card, line) {
			t.Fatalf("bet card missing %q:\n%s", line, card)
		}
	}
}

func TestCardsEscapeHTML(t *testing.T) {
	card := formatTelegram(logger.Record{Level: "ERROR", Message: "bad <price> & stuff", Race: &logger.Race{Venue: "A&B"}})
	if !strings.Contains(card, "bad &lt;price&gt; &amp; stuff") || !strings.Contains(card, "A&amp;B") {
		t.Fatalf("not escaped:\n%s", card)
	}
}
