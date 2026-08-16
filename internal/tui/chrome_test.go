package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #492. The Sessions tab spent 10 rows and 5 full-width rules on chrome, on a
// screen whose only job is to show sessions — 14 rows left for the table on a
// 24-row terminal.
//
// Five identical rules establish no hierarchy: they cut the screen into slices of
// equal importance. And the summary row was largely a restatement of what was
// already on screen — the status counts are the exact aggregate of the STATUS
// column, `hidden 25` and `● ended 25` said the same thing twice on one line, and
// the sort is readable from the arrow in the column header
// (docs/design/sessions-chrome.md § 2).

// chromeModel is a fleet small enough to fit, with no optional chrome line
// showing (no filter, no overflow banner, no failed refresh, no watcher warning).
func chromeModel(width, height int) model {
	m := stubModel()
	m.width, m.height = width, height
	m.sessions = []api.SessionView{
		{ID: "a", Status: "working"},
		{ID: "b", Status: "idle"},
		{ID: "c", Status: "waiting"},
	}
	m.prefs = defaultPrefs()
	m.prefs.hideEnded = false // nothing hidden, so no `hidden N`
	m.sseLive = true
	return m
}

// chromeRows is how many rendered lines are *not* session rows.
func chromeRows(t *testing.T, m model) int {
	t.Helper()
	total := lineCount(m.View())
	return total - len(m.visibleSessions())
}

// The count is the point of the issue, so it is asserted directly. It is 7 until
// #493 folds the key hints into the bottom bar, which is the sixth row.
func TestTheSessionsChromeFitsInSevenRows(t *testing.T) {
	if got := chromeRows(t, chromeModel(300, 40)); got != 7 {
		t.Errorf("the sessions chrome takes %d rows, want 7:\n%s", got, chromeModel(300, 40).View())
	}
}

// Three rules, not five: the one closing the tab labels, the one closing the
// column header, and the one closing the table.
func TestOnlyThreeRulesRemain(t *testing.T) {
	n := 0
	for _, line := range strings.Split(chromeModel(300, 40).View(), "\n") {
		if isRule(line) {
			n++
		}
	}
	if n != 3 {
		t.Errorf("%d full-width rules, want 3:\n%s", n, chromeModel(300, 40).View())
	}
}

// isRule reports whether a rendered line is nothing but rule glyphs.
func isRule(line string) bool {
	stripped := strings.TrimSpace(line)
	if stripped == "" {
		return false
	}
	return strings.Trim(stripped, "─━") == ""
}

// The status counts, the token total and the activity sparkline are gone; what
// the table cannot say by itself is not.
func TestTheSummaryRowIsGone(t *testing.T) {
	out := chromeModel(300, 40).View()
	for _, gone := range []string{"● working 1", "out ", "rc "} {
		if strings.Contains(out, gone) {
			t.Errorf("the summary row still carries %q:\n%s", gone, out)
		}
	}
}

// `hidden N` is the one piece of the summary row that exists nowhere else: `a`
// and `hide idle after` filter silently, and without it the screen claims three
// sessions while the fleet has thirty.
func TestHiddenCountAppearsOnlyWhenSomethingIsHidden(t *testing.T) {
	m := chromeModel(300, 40)
	if strings.Contains(m.View(), "hidden") {
		t.Errorf("nothing is hidden, yet the bar says so:\n%s", m.View())
	}

	m.sessions = append(m.sessions, api.SessionView{ID: "z", Status: "ended"})
	m.prefs.hideEnded = true
	if !strings.Contains(m.View(), "hidden 1") {
		t.Errorf("a hidden session is invisible — the screen claims a smaller fleet than it has:\n%s", m.View())
	}
}

// The connection glyph is the only permanent sign the client is still reaching
// the server (#457). It leaves the deleted summary row for the tab line, and it
// is the last character of it.
func TestTheConnectionGlyphIsTheLastCharacterOfTheTabLine(t *testing.T) {
	m := chromeModel(300, 40)
	first := strings.SplitN(m.View(), "\n", 2)[0]
	if got := lastRune(first); got != "●" {
		t.Errorf("the tab line ends with %q, want the connection glyph:\n%q", got, first)
	}
	m.sseLive, m.err = false, errFake{}
	first = strings.SplitN(m.View(), "\n", 2)[0]
	if got := lastRune(first); got != "○" {
		t.Errorf("with the link down the tab line ends with %q, want ○", got)
	}
}

type errFake struct{}

func (errFake) Error() string { return "unreachable" }

func lastRune(s string) string {
	r := []rune(strings.TrimRight(s, " "))
	if len(r) == 0 {
		return ""
	}
	return string(r[len(r)-1])
}
