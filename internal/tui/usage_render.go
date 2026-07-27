package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-fleet/internal/api"
)

// renderUsage renders the Usage tab: gauges for the 5-hour window and the week.
func renderUsage(u api.UsageReport) string {
	if u.FetchedAt == "" {
		return dimStyle.Render("usage unavailable — the daemon hasn't fetched it yet")
	}
	var b strings.Builder
	b.WriteString(renderGauge("Current session (5h)", u.FiveHourPct, u.FiveHourReset) + "\n\n")
	b.WriteString(renderGauge("This week", u.SevenDayPct, u.SevenDayReset) + "\n\n")
	b.WriteString(dimStyle.Render("synced " + freshness(u.FetchedAt)))
	return b.String()
}

func renderGauge(label string, pct float64, reset string) string {
	const width = 40
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

	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled))
	line := fmt.Sprintf("%-22s %s %3.0f%%", label, bar, pct)
	if reset != "" {
		line += dimStyle.Render("   resets " + resetLabel(reset))
	}
	return line
}

func resetLabel(rfc string) string {
	t, err := parseTime(rfc)
	if err != nil {
		return rfc
	}
	return t.Local().Format("Jan 2 15:04")
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
