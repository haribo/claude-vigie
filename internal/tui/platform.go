package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-fleet/internal/api"
)

var redStyle = lipgloss.NewStyle().Foreground(cRed)

// platformKnown reports whether the server returned a usable indicator, so the
// client shows the indicator and stays silent against a server that does not
// poll platform status (or before the first poll completes).
func platformKnown(ps api.PlatformStatus) bool {
	switch ps.Indicator {
	case "none", "minor", "major", "critical":
		return true
	default:
		return false
	}
}

// platformDisplay maps a Statuspage indicator to a dot, a word, and a style.
func platformDisplay(ps api.PlatformStatus) (dot, word string, style lipgloss.Style) {
	switch ps.Indicator {
	case "none":
		return "●", "operational", statusStyle("working") // green
	case "minor":
		return "●", "degraded", statusStyle("waiting") // amber
	case "major":
		return "●", "major outage", redStyle
	case "critical":
		return "●", "critical outage", redStyle
	default:
		return "○", "unknown", dimStyle
	}
}

// platformStrip renders the usage-strip indicator, e.g. "    platform ●
// operational". It is empty when the server has not reported a usable status.
func platformStrip(ps api.PlatformStatus) string {
	if !platformKnown(ps) {
		return ""
	}
	dot, word, style := platformDisplay(ps)
	return dimStyle.Render("    ") + labelStyle.Render("platform ") + style.Render(dot+" "+word)
}
