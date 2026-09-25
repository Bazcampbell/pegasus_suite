// format.go
//
// Telegram cards. Output is Telegram HTML (see TelegramClient.SendLog, which
// sets the parse mode): HTML rather than Markdown because only < > & need
// escaping, whereas Markdown breaks on any stray _ or * in a venue name or an
// error string.
//
// Two cards, one fact per line:
//
//	formatLog  debug / info / warn / error: the same card, only the icon differs
//	formatBet  bets: the race, then every money figure the bet carries

package telegram

import (
	"fmt"
	"html"
	"strings"

	"pegasus_suite/logger"
)

// formatTelegram picks the card for an entry.
func formatTelegram(e logger.Record) string {
	if e.Bet != nil {
		return formatBet(e)
	}
	return formatLog(e)
}

// formatLog renders a debug, info, warn or error line:
//
//	🟠 WARN · pegasus
//
//	unable to resolve betfair race
//
//	🏇 ROCKHAMPTON R7 · 🐎 #1 Brank Daddy
//	👤 punter · ⚙️ bot-a
func formatLog(e logger.Record) string {
	var sb strings.Builder
	writeHeader(&sb, e)
	writeMessage(&sb, e.Message)

	var lines []string
	if race := raceLine(e.Race, true); race != "" {
		lines = append(lines, race)
	}
	if who := whoLine(e); who != "" {
		lines = append(lines, who)
	}
	writeLines(&sb, lines)

	return sb.String()
}

// formatBet renders a bet:
//
//	🟢 BET · pegasus
//
//	betfair back accepted
//
//	🏇 ROCKHAMPTON R7
//	🐎 Runner #1 Brank Daddy
//	💵 Stake $10.00 @ 4.60 · 🎯 Target $240.00
//	🔌 betfair · BACK · ROCKHAMPTON:R7:1:20260925
//	👤 user-1 · ⚙️ bot-a
func formatBet(e logger.Record) string {
	b := e.Bet

	var sb strings.Builder
	writeHeader(&sb, e)
	writeMessage(&sb, e.Message)

	var lines []string
	if race := raceLine(e.Race, false); race != "" {
		lines = append(lines, race)
	}
	if runner := runnerLine(e.Race); runner != "" {
		lines = append(lines, runner)
	}

	var money []string
	if b.Stake != 0 {
		stake := "💵 Stake " + formatMoney(b.Stake)
		if b.Odds != 0 {
			stake += fmt.Sprintf(" @ %.2f", b.Odds)
		}
		money = append(money, stake)
	}
	if b.Target != 0 {
		money = append(money, "🎯 Target "+formatMoney(b.Target))
	}
	if len(money) > 0 {
		lines = append(lines, strings.Join(money, " · "))
	}

	var source []string
	for _, s := range []string{b.Provider, b.BetType, b.BetID} {
		if s != "" {
			source = append(source, escapeText(s))
		}
	}
	if len(source) > 0 {
		lines = append(lines, "🔌 "+strings.Join(source, " · "))
	}

	if who := whoLine(e); who != "" {
		lines = append(lines, who)
	}
	writeLines(&sb, lines)

	return sb.String()
}

// formatSuppressed summarises repeats that the dedupe window swallowed.
func formatSuppressed(sample logger.Record, count int) string {
	var sb strings.Builder
	writeHeader(&sb, sample)
	fmt.Fprintf(&sb, "\n\n🔁 <b>%d more</b> identical to:\n%s", count, escapeText(strings.TrimSpace(sample.Message)))
	return sb.String()
}

// ---- pieces shared by the cards ----

func writeHeader(sb *strings.Builder, e logger.Record) {
	fmt.Fprintf(sb, "%s <b>%s</b>", levelIcon(e.Level), escapeText(e.Level))
	if e.Application != "" {
		fmt.Fprintf(sb, " · %s", escapeText(e.Application))
	}
}

func writeMessage(sb *strings.Builder, msg string) {
	if msg = strings.TrimSpace(msg); msg != "" {
		fmt.Fprintf(sb, "\n\n%s", escapeText(msg))
	}
}

func writeLines(sb *strings.Builder, lines []string) {
	if len(lines) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(strings.Join(lines, "\n"))
	}
}

// raceLine returns "🏇 VENUE R7", with the runner folded on when withRunner is set.
func raceLine(r *logger.Race, withRunner bool) string {
	if r == nil || r.Venue == "" {
		return ""
	}
	line := "🏇 <b>" + escapeText(r.Venue)
	if r.Number != 0 {
		line += fmt.Sprintf(" R%d", r.Number)
	}
	line += "</b>"
	if withRunner {
		if runner := runnerText(r); runner != "" {
			line += " · 🐎 " + runner
		}
	}
	return line
}

func runnerLine(r *logger.Race) string {
	if runner := runnerText(r); runner != "" {
		return "🐎 Runner " + runner
	}
	return ""
}

func runnerText(r *logger.Race) string {
	if r == nil || (r.Runner == 0 && r.RunnerName == "") {
		return ""
	}
	var parts []string
	if r.Runner != 0 {
		parts = append(parts, fmt.Sprintf("<b>#%d</b>", r.Runner))
	}
	if r.RunnerName != "" {
		parts = append(parts, escapeText(r.RunnerName))
	}
	return strings.Join(parts, " ")
}

// whoLine returns "👤 user · ⚙️ process", whichever are set.
func whoLine(e logger.Record) string {
	var parts []string
	if e.UserID != "" {
		parts = append(parts, "👤 "+escapeText(e.UserID))
	}
	if e.ProcessID != "" {
		parts = append(parts, "⚙️ "+escapeText(e.ProcessID))
	}
	return strings.Join(parts, " · ")
}

func levelIcon(logType string) string {
	switch logType {
	case "ERROR":
		return "🔴"
	case "WARN":
		return "🟠"
	case "BET":
		return "🟢"
	case "INFO":
		return "🔵"
	default:
		return "⚪"
	}
}

func escapeText(s string) string { return html.EscapeString(s) }

func formatMoney(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("$%.2f", v)
}
