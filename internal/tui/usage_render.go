package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
)

// usageStrip is the bottom line — subscription usage plus the platform health
// indicator — kept within width: the TUI never scrolls sideways, so when the two
// do not fit, the secondary platform side is dropped and the usage side is
// clamped as a last resort (#332).
// The Claude platform indicator and the ⟳ sync glyph left this bar for the state
// modal: they are reliability indicators, not figures, and `platform ●
// operational` reads 99 % of the time — a row that trains the eye to skip the
// place where the exception appears (docs/design/sessions-chrome.md § 2, #494).
func usageStrip(u api.UsageReport, width int) string {
	return clampWidth(renderUsageStrip(u), width)
}

// renderUsageStrip renders subscription usage as one compact, dim line for the
// bottom of the Sessions body: short 5h and 7d gauges with % and time-to-reset.
func renderUsageStrip(u api.UsageReport) string {
	if u.FetchedAt == "" {
		return dimStyle.Render("usage — not fetched yet")
	}
	// The space goes inside the gauges, not between them: `5h` and `7d` are two
	// elements of the same nature, already told apart by their own labels, while
	// the bar and its figure have nothing separating them at all (#568).
	weekly := u.SevenDayReset
	strip := labelStyle.Render("usage ") +
		compactGauge("5h", u.FiveHourPct, u.FiveHourReset) +
		dimStyle.Render("  ")
	if u.Scoped == nil {
		return strip + compactGauge("7d", u.SevenDayPct, weekly)
	}
	// The scoped limit shares the seven-day window, so it hangs off that gauge
	// behind a middle dot rather than standing as a third element — and the reset
	// is written once, on the pair, instead of twice.
	//
	// **Once because it is one window, not because it is shorter.** If the endpoint
	// ever reports two instants, both are printed: a single reset would then say
	// something false about one of them. The saving is real either way — it keeps
	// the line inside a narrow terminal, which matters because this strip is
	// clamped and the TUI never scrolls sideways (#332, #840).
	shared := u.Scoped.Reset == weekly
	sevenDayReset := weekly
	if shared {
		sevenDayReset = ""
	}
	return strip +
		compactGauge("7d", u.SevenDayPct, sevenDayReset) +
		dimStyle.Render(" · ") +
		compactGauge(strings.ToLower(u.Scoped.Label), u.Scoped.Pct, u.Scoped.Reset)
}

// Amber from 50 %, red from 80 % — the levels docs/design/usage.md § 1bis fixes
// for every client, and the browser's `usageLevel` is the same rule in lib.js.
//
// Not the Ctx column's 60/85 (context.go): a context at 85 % frees itself in a
// compaction and the session carries on, while a usage limit at 85 % clears only
// by waiting, possibly for days. The choice the operator still has — a smaller
// model, or stopping — has to be offered before it fills (#844).
const (
	usageAmberFrom = 50.0
	usageRedFrom   = 80.0
)

func usageColor(pct float64) lipgloss.AdaptiveColor {
	switch {
	case pct >= usageRedFrom:
		return cRed
	case pct >= usageAmberFrom:
		return cAmber
	default:
		return cGreen
	}
}

func compactGauge(label string, pct float64, reset string) string {
	const width = 10
	filled := int(pct / 100 * float64(width))
	switch {
	case filled > width:
		filled = width
	case filled < 0:
		filled = 0
	}
	color := usageColor(pct)
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("▓", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled))
	// The reset stays tight against the percentage — it qualifies that figure, and
	// a space would make it read as a third element. The bar does not: the fill
	// glyph and the digit carry the same visual weight, so `░░4%` was one block
	// and the eye had to hunt for where the bar ended (#568).
	//
	// #492 tightened both, on the grounds that the gauges give up the space the
	// chrome no longer has to spare. That holds for the space *between* the two
	// gauges, which is where it was taken back from; it did not hold here. The
	// `%3.0f` padding that kept the 5h and 7d blocks aligned whatever the figure
	// is still gone — that part of the trade stands.
	s := labelStyle.Render(label+" ") + bar + " " + fmt.Sprintf("%.0f%%", pct)
	if r := resetIn(reset); r != "" {
		s += dimStyle.Render("(" + r + ")")
	}
	return s
}

// resetIn renders the time remaining until a reset (compact), or "" if unknown.
func resetIn(rfc string) string {
	t, err := parseTime(rfc)
	if err != nil {
		return ""
	}
	if d := clock.Until(t); d > 0 {
		return humanizeDuration(d)
	}
	return "now"
}

// parseTime accepts RFC3339 with or without fractional seconds (the endpoint
// returns microseconds).
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
