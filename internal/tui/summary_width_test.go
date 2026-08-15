package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// #486. The summary strip's two halves did not share a width budget:
// renderSummaryFit grew the left block to whatever the *full* terminal width
// allowed, then joinLR found no room for the right block and dropped it whole.
// Below ~140 columns the operator lost `sort`, `group`, `hidden` and the
// server-connection glyph, with nothing on screen to say they had been cut.
//
// The glyph is the load-bearing one: since #457 it is the TUI's only permanent
// indication that it is still talking to the server. The left block's extras
// (`out`, `rc`, `activity`) are decorative beside it, and they were winning.

// summaryFleet is the fleet the issue was measured against: five active
// sessions across the statuses that widen the counts, plus enough ended ones to
// make `hidden` non-zero.
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
	m.prefs.hideEnded = true // so hiddenCount() is non-zero, as when it was observed
	m.sseLive = true
	return m
}

// The assertion is on the assembled line, not on the two halves: the defect
// lives in how they are put together, so checking them separately would have
// passed throughout.
func TestTheSummaryKeepsTheConnectionGlyphOnANarrowTerminal(t *testing.T) {
	for _, width := range []int{100, 107, 120, 139} {
		line := summaryModel(width).summaryStrip()
		if !strings.Contains(line, "●") {
			t.Errorf("width %d: the connection glyph is gone — the operator has no signal left when the stream drops:\n%s", width, line)
		}
		if !strings.Contains(line, "hidden") {
			t.Errorf("width %d: `hidden` is gone — nothing says the list is filtered:\n%s", width, line)
		}
		if got := lipgloss.Width(line); got > width {
			t.Errorf("width %d: the strip renders %d columns wide and would overflow", width, got)
		}
	}
}

// The right block is reserved first, but the left block must still get every
// extra a wide terminal can afford — the fix must not cost anything at width.
func TestAWideTerminalStillShowsEverything(t *testing.T) {
	line := summaryModel(200).summaryStrip()
	for _, want := range []string{"●", "hidden", "sort"} {
		if !strings.Contains(line, want) {
			t.Errorf("a 200-column strip is missing %q:\n%s", want, line)
		}
	}
	if got := lipgloss.Width(line); got != 200 {
		t.Errorf("the strip is %d columns wide on a 200-column terminal, want 200 (right-aligned)", got)
	}
}

// An unknown width (before the first WindowSizeMsg) must keep rendering
// everything unbounded, as it did — a reserved-width calculation on width 0 must
// not silently blank the strip.
func TestAnUnknownWidthStillRendersBothHalves(t *testing.T) {
	line := summaryModel(0).summaryStrip()
	for _, want := range []string{"●", "hidden", "sort"} {
		if !strings.Contains(line, want) {
			t.Errorf("with no width yet, the strip is missing %q:\n%s", want, line)
		}
	}
}

// Reserving the right block shrinks the left one, which made the counts clamp
// where they used to fit — and clamping cuts a number in half: `ended 25` became
// `ended 2`, a wrong figure shown as a right one. Whatever the width, a count on
// screen must be the real count; an entry that does not fit is dropped whole.
func TestNoWidthEverShowsAWrongCount(t *testing.T) {
	truth := map[string]string{"call": "1", "working": "2", "waiting": "1", "idle": "1", "stalled": "1", "ended": "25"}
	shown := regexp.MustCompile(`[●◌] ([a-z]+) (\d+)`)

	for width := 40; width <= 200; width++ {
		line := summaryModel(width).summaryStrip()
		for _, m := range shown.FindAllStringSubmatch(line, -1) {
			want, known := truth[m[1]]
			if !known {
				t.Errorf("width %d: unexpected count %q in the strip:\n%s", width, m[1], line)
				continue
			}
			if m[2] != want {
				t.Errorf("width %d: the strip says %s %s, but there are %s:\n%s", width, m[1], m[2], want, line)
			}
		}
	}
}

// A truncated list must say it is truncated, or the operator reads a short list
// as a complete one.
func TestATruncatedCountListSaysSo(t *testing.T) {
	full := summaryModel(200).summaryStrip()
	if strings.Contains(full, "…") {
		t.Fatalf("a 200-column strip should need no ellipsis:\n%s", full)
	}
	cut := summaryModel(107).summaryStrip()
	if !strings.Contains(cut, "ended") && !strings.Contains(cut, "…") {
		t.Errorf("counts were dropped with nothing to show for it:\n%s", cut)
	}
}

// The vertical budget measures the same string the view renders. If the two ever
// diverge the table is sized against a strip nobody sees (#378).
func TestTheRowBudgetMeasuresTheStripThatIsRendered(t *testing.T) {
	m := summaryModel(107)
	m.height = 40
	if !strings.Contains(m.viewSessions(), strings.TrimRight(m.summaryStrip(), " ")) {
		t.Error("the rendered view does not contain the strip the budget measures")
	}
}
