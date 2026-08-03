package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// contextWindow returns a model's context window in tokens. Opus/Sonnet at 4.6+
// and anything 5+ carry 1M; Fable 5 carries 1M; Haiku and older models carry
// 200K; an unknown model gets the conservative 200K (#279). Maintained by hand,
// like shortModel — a wrong guess only skews a percentage, never a status.
func contextWindow(model string) int64 {
	const big, base = 1_000_000, 200_000
	family, major, minor := modelVersion(shortModel(model))
	switch family {
	case "fable":
		return big
	case "opus", "sonnet":
		if major > 4 || (major == 4 && minor >= 6) {
			return big
		}
		return base
	default: // haiku, unknown, empty
		return base
	}
}

// modelVersion splits a short model name ("opus-4-8", "sonnet-5") into its family
// and major/minor version; missing parts are 0.
func modelVersion(short string) (family string, major, minor int) {
	parts := strings.Split(short, "-")
	family = parts[0]
	if len(parts) > 1 {
		major, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		minor, _ = strconv.Atoi(parts[2])
	}
	return family, major, minor
}

// contextPct is how full a session's context window is, 0 when unknown.
func contextPct(s api.SessionView) float64 {
	if s.ContextTokens <= 0 {
		return 0
	}
	return float64(s.ContextTokens) / float64(contextWindow(s.Model)) * 100
}

// contextColor maps a context-fill % to green / amber / red: green < 60 ≤ amber
// < 85 ≤ red. Not "% before auto-compact" — Claude Code's compaction threshold is
// undocumented, so this is fill of the window, not distance to compaction (#279).
func contextColor(pct float64) lipgloss.AdaptiveColor {
	switch {
	case pct >= 85:
		return cRed
	case pct >= 60:
		return cAmber
	default:
		return cGreen
	}
}

// contextCell renders the table CTX cell: the fill percentage, or "-" when unknown.
func contextCell(s api.SessionView) string {
	if s.ContextTokens <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", contextPct(s))
}

// contextGauge renders a compact context-fill bar for the detail panel.
func contextGauge(s api.SessionView) string {
	if s.ContextTokens <= 0 {
		return dimStyle.Render("unknown")
	}
	pct := contextPct(s)
	const width = 10
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	} else if filled < 0 {
		filled = 0
	}
	bar := lipgloss.NewStyle().Foreground(contextColor(pct)).Render(strings.Repeat("▓", filled)) +
		dimStyle.Render(strings.Repeat("░", width-filled))
	return fmt.Sprintf("%s %3.0f%% ", bar, pct) + dimStyle.Render(fmt.Sprintf("(%s / %s)",
		humanizeTokens(s.ContextTokens), humanizeTokens(contextWindow(s.Model))))
}
