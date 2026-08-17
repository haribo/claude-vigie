package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
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
	return labelStyle.Render("usage  ") +
		compactGauge("5h", u.FiveHourPct, u.FiveHourReset) +
		dimStyle.Render("    ") +
		compactGauge("7d", u.SevenDayPct, u.SevenDayReset)
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
	color := cGreen
	switch {
	case pct >= 80:
		color = cRed
	case pct >= 50:
		color = cAmber
	}
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("▓", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled))
	// Tight against the bar and against the reset: the gauges give up the space
	// the chrome no longer has to spare (#492). The `%3.0f` padding that kept the
	// 5h and 7d blocks aligned whatever the figure goes with it — a deliberate
	// reading trade, not an oversight (docs/design/sessions-chrome.md § 2).
	s := labelStyle.Render(label+" ") + bar + fmt.Sprintf("%.0f%%", pct)
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
	if d := time.Until(t); d > 0 {
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
