package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// #486, carried over to the bottom bar. The two halves of a chrome row must
// share one width budget: the right half used to be measured last and dropped
// whole when it did not fit, so below ~140 columns the operator silently lost the
// view state. The summary row this was written against is gone (#492), but the
// bar that replaced it joins two halves the same way, and the failure mode is
// identical.
//
// `hidden N` is what makes it matter now: it is the only thing on screen saying
// the list is filtered, so losing it means the screen claims a smaller fleet than
// there is (docs/design/sessions-chrome.md § 2).

func summaryFleet() []api.SessionView {
	out := []api.SessionView{
		{ID: "a", Status: "working", CallAt: "2026-08-15T12:00:00Z"},
		{ID: "b", Status: "working"},
		{ID: "c", Status: "waiting"},
		{ID: "d", Status: "idle"},
		{ID: "e", Status: "stalled"},
	}
	for i := 0; i < 25; i++ {
		out = append(out, api.SessionView{ID: string(rune('A'+i)) + "-ended", Status: "ended"})
	}
	return out
}

func summaryModel(width int) model {
	m := stubModel()
	m.width = width
	m.sessions = summaryFleet()
	m.prefs = defaultPrefs()
	m.prefs.hideEnded = true // 25 hidden, as when the defect was observed
	m.sseLive = true
	m.usage = api.UsageReport{
		FiveHourPct: 28, FiveHourReset: "2026-08-15T16:00:00Z",
		SevenDayPct: 69, SevenDayReset: "2026-08-18T15:00:00Z",
		FetchedAt: "2026-08-15T15:00:00Z",
	}
	return m
}

// The assertion is on the assembled line, not on the two halves: the defect
// lives in how they are put together, so checking them separately would have
// passed throughout.
func TestTheBottomBarKeepsTheViewStateOnANarrowTerminal(t *testing.T) {
	for _, width := range []int{80, 100, 107, 120, 139} {
		line := summaryModel(width).bottomBar()
		if !strings.Contains(line, "hidden") {
			t.Errorf("width %d: `hidden` is gone — nothing says the list is filtered:\n%s", width, line)
		}
		if got := lipgloss.Width(line); got > width {
			t.Errorf("width %d: the bar renders %d columns wide and would overflow", width, got)
		}
	}
}

// The right half is reserved first, but a wide terminal must still show
// everything — the budget must not cost anything at width.
func TestAWideTerminalStillShowsEverything(t *testing.T) {
	line := summaryModel(200).bottomBar()
	for _, want := range []string{"usage", "sort", "hidden 25"} {
		if !strings.Contains(line, want) {
			t.Errorf("a 200-column bar is missing %q:\n%s", want, line)
		}
	}
	if got := lipgloss.Width(line); got != 200 {
		t.Errorf("the bar is %d columns wide on a 200-column terminal, want 200 (right-aligned)", got)
	}
}

// An unknown width (before the first WindowSizeMsg) must keep rendering
// everything unbounded — a reserved-width calculation on width 0 must not
// silently blank the bar.
func TestAnUnknownWidthStillRendersBothHalves(t *testing.T) {
	line := summaryModel(0).bottomBar()
	for _, want := range []string{"usage", "sort", "hidden"} {
		if !strings.Contains(line, want) {
			t.Errorf("with no width yet, the bar is missing %q:\n%s", want, line)
		}
	}
}

// The vertical budget measures the same string the view renders. If the two ever
// diverge the table is sized against a bar nobody sees (#378).
func TestTheRowBudgetMeasuresTheBarThatIsRendered(t *testing.T) {
	m := summaryModel(107)
	m.height = 40
	if !strings.Contains(m.viewSessions(), strings.TrimRight(m.bottomBar(), " ")) {
		t.Error("the rendered view does not contain the bar the budget measures")
	}
}
