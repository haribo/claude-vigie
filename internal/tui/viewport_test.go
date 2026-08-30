package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestWindow(t *testing.T) {
	cases := []struct {
		name                        string
		n, selected, budget, offset int
		wantStart, wantEnd, wantOff int
		wantScrolled                bool
	}{
		{"fits exactly", 10, 3, 10, 0, 0, 10, 0, false},
		{"fits under", 5, 2, 10, 0, 0, 5, 0, false},
		{"top edge clamps to 0", 100, 0, 10, 50, 0, 10, 0, true},
		{"bottom edge clamps to max", 100, 99, 10, 0, 90, 100, 90, true},
		{"sticky: cursor inside band keeps offset", 100, 50, 10, 45, 45, 55, 45, true},
		{"scroll down by one past margin", 100, 53, 10, 45, 46, 56, 46, true},
		{"scroll up past margin", 100, 44, 10, 45, 42, 52, 42, true},
		{"resize re-clamps a stale huge offset", 100, 50, 10, 999, 48, 58, 48, true},
	}
	for _, c := range cases {
		start, end, off, scrolled := window(c.n, c.selected, c.budget, c.offset)
		if start != c.wantStart || end != c.wantEnd || off != c.wantOff || scrolled != c.wantScrolled {
			t.Errorf("%s: window(%d,%d,%d,%d) = (%d,%d,%d,%v), want (%d,%d,%d,%v)",
				c.name, c.n, c.selected, c.budget, c.offset,
				start, end, off, scrolled, c.wantStart, c.wantEnd, c.wantOff, c.wantScrolled)
		}
		// The selected row must always land inside the returned window.
		if c.selected >= start && c.selected < end {
			continue
		}
		t.Errorf("%s: selected %d not visible in [%d,%d)", c.name, c.selected, start, end)
	}
}

func TestLineCount(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{{"", 0}, {"one", 1}, {"a\nb", 2}, {"a\nb\nc", 3}} {
		if got := lineCount(c.in); got != c.want {
			t.Errorf("lineCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestSessionsViewportFitsHeight is the #378 height dual of the width sweep: for
// a populated fleet with the cursor on the last row, the sessions view must fit
// the terminal height, keep the cursor visible, and show the scroll indicator
// exactly when the list overflows the band.
func TestSessionsViewportFitsHeight(t *testing.T) {
	const heightFloor, heightCeil, width = 22, 60, 120
	m := sampleModel(tabSessions)
	m.width = width
	m.sess.cursor = len(m.visibleSessions()) - 1 // worst case: last row must be reachable
	m = m.scrollToCursor()

	for h := heightFloor; h <= heightCeil; h++ {
		m.height = h
		m = m.scrollToCursor()
		view := m.View()
		lines := strings.Count(view, "\n") + 1
		if lines > h {
			t.Fatalf("height %d: rendered %d lines (overflow)", h, lines)
		}
		if !strings.Contains(view, "▎") {
			t.Errorf("height %d: cursor row not visible", h)
		}
		// The pinned column header must always be present, whatever the height:
		// a windowed table whose header scrolled away is unreadable.
		//
		// This used to assert `working` under the message "summary counts
		// missing". The summary it was named for was deleted in #492, and the
		// string went on matching a STATUS cell of one of the three working
		// sessions in the fixture — so it passed for a reason unrelated to what it
		// claimed, and would have passed with no header at all. A column header is
		// something that still exists and that the spec still requires (#556).
		if !strings.Contains(view, "STATUS") {
			t.Errorf("height %d: the pinned column header is missing", h)
		}
		// The indicator appears iff the body was actually windowed.
		_, budget := m.sessionsBand(m.bodyHeight())
		hasIndicator := strings.Contains(view, "rows ")
		if wantIndicator := budget > 0; hasIndicator != wantIndicator {
			t.Errorf("height %d: indicator=%v, want %v (budget=%d)", h, hasIndicator, wantIndicator, budget)
		}
	}
}

// #650. The two banners this tab used to draw were measured into the row budget —
// one of them wrongly: `staleReason` was written by the sessions view and never
// counted, so a failed refresh made the screen render one row past the terminal.
//
// They are gone, and this keeps the budget honest in the states that used to draw
// them: whatever is on screen, the tab fits the terminal exactly.
func TestTheTabFitsTheTerminalInEveryFailedState(t *testing.T) {
	sessions := make([]api.SessionView, 40)
	for i := range sessions {
		sessions[i] = api.SessionView{
			ID: fmt.Sprintf("s%02d", i), Name: fmt.Sprintf("s%02d", i),
			Machine: "orion", Status: "idle", LastSeenAt: "2026-08-30T12:00:00Z",
		}
	}
	for _, c := range []struct {
		name    string
		failed  bool
		watcher bool
	}{
		{"healthy", false, false},
		{"a refresh failed", true, false},
		{"a watcher stopped", false, true},
		{"both", true, true},
	} {
		m := stubModel()
		m.width, m.height = 140, 24
		m = m.applySessions(sessionsMsg{gen: 1, sessions: sessions})
		if c.failed {
			m.err = errors.New("context deadline exceeded")
			m.markRefresh(srcSessions, m.err)
		}
		if c.watcher {
			m.gotWatcher = true
			m.watcherMachines = map[string]string{"orion": "2020-01-01T00:00:00Z"}
		}
		if got := lineCount(m.View()); got > m.height {
			t.Errorf("%s: the tab rendered %d lines into a %d-row terminal", c.name, got, m.height)
		}
	}
}
