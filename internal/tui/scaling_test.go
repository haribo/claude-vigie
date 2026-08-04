package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// The TUI never scrolls sideways, so no rendered line may be wider than the
// terminal. These tests sweep a range of widths and assert that invariant per
// tab — the reusable scaling guard (#328). The floor is 72: below ~56 the
// mandatory NAME/DIR/STATUS columns cannot fit, a distinct extreme not in scope.
const scaleFloor, scaleCeil = 72, 200

// sampleModel is a small, representative model for a given tab.
func sampleModel(t tab) model {
	return model{
		tab:   t,
		prefs: defaultPrefs(),
		sessions: []api.SessionView{
			{Title: "alpha", Machine: "minet", User: "nico", Status: "working", Activity: "editing render.go", LastSeenAt: "2026-07-26T10:00:00Z", Usage: api.Usage{OutputTokens: 1234}},
			{Title: "beta", Machine: "srv", User: "nico", Status: "waiting", Activity: "waiting on approval", LastSeenAt: "2026-07-26T11:00:00Z"},
		},
	}
}

// assertNoOverflow renders m across the width sweep and fails on any line wider
// than the terminal.
func assertNoOverflow(t *testing.T, m model) {
	t.Helper()
	for w := scaleFloor; w <= scaleCeil; w++ {
		m.width = w
		for i, ln := range strings.Split(m.View(), "\n") {
			if lw := lipgloss.Width(ln); lw > w {
				t.Errorf("width %d: line %d overflows by %d cols: %q", w, i, lw-w, ln)
			}
		}
	}
}

// TestSessionsTabNeverOverflowsWidth is the #328 regression: across the width
// sweep, no line in the Sessions view (summary strip, table, footer) exceeds the
// terminal width.
func TestSessionsTabNeverOverflowsWidth(t *testing.T) {
	assertNoOverflow(t, sampleModel(tabSessions))
}
