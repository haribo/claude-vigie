package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// #493, superseding #487. Eleven key hints needed 134 columns, so a narrower
// terminal got a row that was both permanently occupied and incomplete — the
// trailing hints dropped with an ellipsis. Eleven reminders are never needed at
// once: one stays on screen and the rest live behind it
// (docs/design/sessions-chrome.md § 5bis).

func helpModel(width int) model {
	m := chromeModel(width, 40)
	return m
}

// The wrap this closes for good: whatever the width, the hint costs one row and
// is never cut.
func TestTheHintCostsOneRowAtEveryWidth(t *testing.T) {
	for _, width := range []int{60, 80, 100, 120, 200} {
		m := helpModel(width)
		bar := m.bottomBar()
		if lineCount(bar) != 1 {
			t.Errorf("width %d: the bottom bar takes %d rows:\n%s", width, lineCount(bar), bar)
		}
		if !strings.Contains(bar, "help") {
			t.Errorf("width %d: the way to the shortcuts is not on screen:\n%s", width, bar)
		}
		if strings.Contains(bar, "…") {
			t.Errorf("width %d: the bar is truncated:\n%s", width, bar)
		}
	}
}

// With the hints folded into the bar, the Sessions chrome reaches the six rows
// the design targets.
func TestTheSessionsChromeFitsInSixRows(t *testing.T) {
	if got := chromeRows(t, chromeModel(300, 40)); got != 6 {
		t.Errorf("the sessions chrome takes %d rows, want 6:\n%s", got, chromeModel(300, 40).View())
	}
}

// h opens it, h closes it, esc closes it.
func TestTheModalOpensAndClosesOnItsOwnKeys(t *testing.T) {
	press := func(m model, key string) model {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		return next.(model)
	}
	m := helpModel(120)
	m = press(m, "h")
	if !m.showHelp {
		t.Fatal("h did not open the shortcuts modal")
	}
	if !strings.Contains(m.View(), "Shortcuts") {
		t.Errorf("the modal is open but not drawn:\n%s", m.View())
	}
	if m2 := press(m, "h"); m2.showHelp {
		t.Error("h did not close the modal")
	}
	esc, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if esc.(model).showHelp {
		t.Error("esc did not close the modal")
	}
}

// A key pressed at a list of keys must not also act on the table behind it.
func TestTheModalSwallowsNavigationKeys(t *testing.T) {
	m := helpModel(120)
	m.showHelp = true
	m.sess.cursor = 1
	before := m.sess

	for _, key := range []string{"j", "k", "g", "s", "S", "/", "n", "r"} {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		got := next.(model)
		if got.sess.cursor != before.cursor || got.sess.groupBy != before.groupBy ||
			got.sess.sortKey != before.sortKey || got.sess.sortReversed != before.sortReversed ||
			got.sess.filtering != before.filtering || got.sess.detail != before.detail {
			t.Errorf("%q acted on the table while the modal was open", key)
		}
		if !got.showHelp {
			t.Errorf("%q closed the modal", key)
		}
	}
	// The way out of the program still works from anywhere.
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q must still quit with the modal open")
	}
}

// Every tab gets its own list, and the help key is reachable from all of them.
func TestEveryTabHasItsOwnShortcutList(t *testing.T) {
	for _, c := range []struct {
		tab  tab
		want string
	}{
		{tabSessions, "next attention"},
		{tabStats, "period"},
		{tabSettings, "change"},
		{tabMachines, "refresh"},
	} {
		out := renderHelp(c.tab, 120)
		if !strings.Contains(out, c.want) {
			t.Errorf("tab %v: the modal does not list %q:\n%s", c.tab, c.want, out)
		}
		if !strings.Contains(out, "esc to close") {
			t.Errorf("tab %v: the modal does not say how to leave it", c.tab)
		}
	}
}

// The `a` binding rewrote tui.toml from a bare unmodified letter, with no
// confirmation and no undo, while the same setting sits one tab away under a
// readable label. It is gone; the modal must not advertise it either.
func TestTheHideEndedBindingIsGone(t *testing.T) {
	m := helpModel(120)
	before := m.prefs.hideEnded
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if next.(model).prefs.hideEnded != before {
		t.Error("`a` still flips hideEnded — a bare letter that rewrites the preferences file")
	}
	if strings.Contains(renderHelp(tabSessions, 120), "hide ended") {
		t.Error("the modal still advertises a binding that no longer exists")
	}
}
