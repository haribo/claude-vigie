package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// contextKnown reports whether the session carries a real context reading (a
// non-nil count, including a known 0), versus none at all — rendered "-" (#367).
//
// Both pointers are required, and neither is taken as proof of the other. The
// daemon sets them together (ADR-0011) and the fleet's drift gate keeps client
// and daemon on one build, but a render loop is the wrong place to trust an
// invariant it does not enforce: the cost of being wrong is a nil dereference in
// the middle of drawing the board.
func contextKnown(s api.SessionView) bool {
	return s.ContextTokens != nil && s.ContextPct != nil
}

// contextPct is how full a session's context window is, 0 when there is no
// reading. The daemon derived it (ADR-0011); this only widens it for the gauge
// arithmetic and the color thresholds.
func contextPct(s api.SessionView) float64 {
	if s.ContextPct == nil {
		return 0
	}
	return float64(*s.ContextPct)
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

// contextCell renders the table CTX cell: the fill percentage (0% for a known
// empty context, e.g. a just-cleared session), or "-" when there is no reading.
func contextCell(s api.SessionView) string {
	if !contextKnown(s) {
		return "-"
	}
	return fmt.Sprintf("%d%%", *s.ContextPct)
}

// contextGauge renders a compact context-fill bar for the detail panel.
func contextGauge(s api.SessionView) string {
	if !contextKnown(s) {
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
		humanizeTokens(*s.ContextTokens), humanizeTokens(s.ContextWindow)))
}
