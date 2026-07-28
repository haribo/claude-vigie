package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-fleet/internal/api"
)

// renderUsageStrip renders subscription usage as one compact, dim line for the
// bottom of the Sessions body: short 5h and 7d gauges with % and time-to-reset.
func renderUsageStrip(u api.UsageReport) string {
	if u.FetchedAt == "" {
		return dimStyle.Render("usage — not fetched yet")
	}
	return labelStyle.Render("usage  ") +
		compactGauge("5h", u.FiveHourPct, u.FiveHourReset) +
		dimStyle.Render("    ") +
		compactGauge("7d", u.SevenDayPct, u.SevenDayReset) +
		dimStyle.Render("    synced "+freshness(u.FetchedAt))
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
	s := labelStyle.Render(label+" ") + bar + fmt.Sprintf(" %3.0f%%", pct)
	if r := resetIn(reset); r != "" {
		s += dimStyle.Render(" (" + r + ")")
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

func freshness(rfc string) string {
	t, err := parseTime(rfc)
	if err != nil {
		return rfc
	}
	switch d := time.Since(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// parseTime accepts RFC3339 with or without fractional seconds (the endpoint
// returns microseconds).
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
