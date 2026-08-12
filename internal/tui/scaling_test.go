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

// sampleModel is a realistic, fully-populated model for a given tab: enough
// sessions for a wide summary, plus history/usage/platform so the strips that
// only render with data (the activity sparkline, the bottom usage/platform
// strip) are actually exercised by the sweep (#332).
func sampleModel(t tab) model {
	var sessions []api.SessionView
	add := func(title, dir, status string, out int64, rc bool) {
		sessions = append(sessions, api.SessionView{
			Title: title, Machine: "minet-dev", User: "nico", ProjectDir: "/home/nico/" + dir,
			Status: status, RemoteControl: rc, Detail: "editing " + dir,
			LastSeenAt: "2026-08-04T15:00:00Z", Usage: api.Usage{OutputTokens: out},
		})
	}
	add("plain-note", "note", "working", 1_280_100_000, true)
	add("melonia", "nomsters", "working", 2_821_800_000, true)
	add("shellf", "shellf", "working", 1_421_500_000, true)
	for i := 0; i < 7; i++ {
		add("idle-"+string(rune('a'+i)), "raccoon", "idle", int64(i)*1000, i < 4)
	}
	for i := 0; i < 12; i++ {
		add("ended-"+string(rune('a'+i)), "claude", "ended", int64(i)*1_000_000, false)
	}
	return model{
		tab:      t,
		prefs:    defaultPrefs(),
		sessions: sessions,
		sess:     sessionsView{history: []int{0, 1, 2, 1, 3, 2, 4, 3, 2, 1, 2, 3}}, // populates the "activity" sparkline
		usage: api.UsageReport{
			FiveHourPct: 28, FiveHourReset: "2026-08-04T16:00:00Z",
			SevenDayPct: 69, SevenDayReset: "2026-08-07T15:00:00Z",
			FetchedAt: "2026-08-04T15:00:00Z",
		},
		platform:      api.PlatformStatus{Indicator: "none", Description: "All Systems Operational", FetchedAt: "2026-08-04T15:00:00Z"},
		daemonVersion: api.VersionInfo{Version: "0.2.0", Commit: "90a5c39", BuildTime: "2026-08-05T08:23:08Z"}, // exercises the Build detail line (#341)
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

// TestMachinesTabNeverOverflowsWidth and TestSettingsTabNeverOverflowsWidth
// extend the scaling guard to the remaining tabs (#329).
func TestMachinesTabNeverOverflowsWidth(t *testing.T) {
	m := sampleModel(tabMachines)
	m.watcherMachines = map[string]string{"minet": "2026-07-26T10:00:00Z"}
	assertNoOverflow(t, m)
}

func TestSettingsTabNeverOverflowsWidth(t *testing.T) {
	assertNoOverflow(t, sampleModel(tabSettings))
}

// TestSummaryDropsWholeElementsWhenNarrow is the #334 regression: the summary
// clamps without overflowing, but at a width too narrow for the activity element
// it must drop it whole rather than show a mid-glyph cut. Width 88 fits the
// counts + out + rc (~76) but not the ~93-wide full line.
func TestSummaryDropsWholeElementsWhenNarrow(t *testing.T) {
	m := sampleModel(tabSessions)

	m.width = 300 // wide: the activity element is present
	if !strings.Contains(strings.Split(m.viewSessions(), "\n")[0], "activity") {
		t.Fatal("activity should show when there is room")
	}

	m.width = 88 // narrow: activity does not fit and must be dropped whole
	line := strings.Split(m.viewSessions(), "\n")[0]
	if strings.Contains(line, "activ") {
		t.Errorf("narrow summary shows a truncated activity element: %q", line)
	}
	if !strings.Contains(line, "working") {
		t.Errorf("status counts must always survive: %q", line)
	}
	if lipgloss.Width(line) > m.width {
		t.Errorf("summary overflows: %d > %d", lipgloss.Width(line), m.width)
	}
}
