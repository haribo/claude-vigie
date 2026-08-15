package tui

import (
	"strings"
	"testing"
)

// #487. The Sessions footer needs 134 columns for its eleven hints. footerBlock
// renders it through lipgloss's Width, which wraps rather than truncates, so
// every terminal narrower than that spent a second row on it — permanently, on
// every frame. bodyHeight measures the footer correctly, so nothing was drawn out
// of place; the row was simply gone. On a 24-row terminal that is two sessions
// traded for a standing reminder of `q quit`.

// The defect, at the width it was reported at and across the range a terminal
// realistically takes.
func TestTheFooterNeverSpendsASecondRow(t *testing.T) {
	for _, width := range []int{60, 80, 100, 107, 120, 133, 134, 200} {
		m := stubModel()
		m.width = width
		if got := lineCount(m.footerBlock()); got != 1 {
			t.Errorf("width %d: the footer takes %d rows:\n%s", width, got, m.footerBlock())
		}
	}
}

// Whatever is dropped, the way out stays on screen.
func TestTheQuitHintSurvivesEveryWidth(t *testing.T) {
	for width := 40; width <= 200; width++ {
		if line := footerFit(tabSessions, width); !strings.Contains(line, "quit") {
			t.Fatalf("width %d: `quit` was dropped:\n%s", width, line)
		}
	}
}

// A footer with hints missing must say so, or the operator reads a short list as
// the complete set of what the tab can do.
func TestATruncatedFooterSaysSo(t *testing.T) {
	if full := footerFit(tabSessions, 200); strings.Contains(full, "…") {
		t.Errorf("a 200-column footer should need no ellipsis:\n%s", full)
	}
	if cut := footerFit(tabSessions, 100); !strings.Contains(cut, "…") {
		t.Errorf("hints were dropped with nothing to show for it:\n%s", cut)
	}
}

// A terminal wide enough must still show every hint — the fix must cost nothing
// at width.
func TestAWideTerminalKeepsEveryHint(t *testing.T) {
	line := footerFit(tabSessions, 200)
	for _, want := range []string{"switch", "select", "detail", "next", "filter", "sort", "reverse", "group", "hide ended", "refresh", "quit"} {
		if !strings.Contains(line, want) {
			t.Errorf("a 200-column footer is missing %q:\n%s", want, line)
		}
	}
}

// The other tabs are narrower and were never affected, but they go through the
// same path now — a regression there would be silent.
func TestTheOtherTabsAreUnchangedAtWidth(t *testing.T) {
	for _, tb := range []tab{tabSettings, tabStats, tabMachines} {
		if got, want := footerFit(tb, 200), footer(tb); got != want {
			t.Errorf("tab %v: fitting changed a footer that already fits:\n got %s\nwant %s", tb, got, want)
		}
	}
}
