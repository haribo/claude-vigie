package tui

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-vigie/internal/api"
)

// The Sessions tab's own behavior lives here (#379): the state transitions the
// tab owns — filtering, sorting/grouping selection, cursor and detail movement —
// operate on sessionsView alone. Anything that needs the terminal geometry (the
// viewport offset) stays on the model, which re-scrolls after a transition; this
// file holds no rendering.

// visible returns the sessions to show: filtered, then sorted, then grouped, with
// ended and idle-aged sessions hidden per the persistent view prefs.
func (v sessionsView) visible(all []api.SessionView, p prefs, now time.Time) []api.SessionView {
	out := make([]api.SessionView, 0, len(all))
	for _, s := range all {
		if !p.visible(s, now) {
			continue
		}
		if v.matchesFilter(s) {
			out = append(out, s)
		}
	}
	sortSessions(out, v.sortKey, v.sortReversed)
	if v.groupBy != groupNone {
		gb := v.groupBy
		sort.SliceStable(out, func(i, j int) bool {
			return groupKey(out[i], gb) < groupKey(out[j], gb)
		})
	}
	return out
}

// matchesFilter reports whether a session passes the active filter: the special
// "rc" token matches remote-controlled sessions, everything else is a fuzzy
// subsequence match over the session's text.
func (v sessionsView) matchesFilter(s api.SessionView) bool {
	if v.filter == "" {
		return true
	}
	if strings.EqualFold(v.filter, "rc") {
		return s.RemoteControl
	}
	return fuzzyMatch(v.filter, sessionHaystack(s))
}

// cursorForSelection returns the cursor index that keeps selectedID under the
// cursor after a reorder, clamping if the session is gone or none is pinned.
func (v sessionsView) cursorForSelection(vis []api.SessionView) int {
	if v.selectedID != "" {
		for i, s := range vis {
			if s.ID == v.selectedID {
				return i
			}
		}
	}
	return clamp(v.cursor, len(vis))
}

// handleNav applies a navigation key to the tab state given the current visible
// list. It leaves the viewport offset alone — the model re-scrolls after, since
// the offset depends on the terminal geometry the model owns (#378/#379).
func (v sessionsView) handleNav(msg tea.KeyMsg, vis []api.SessionView) sessionsView {
	switch msg.String() {
	case "down", "j":
		if v.detail { // in detail, ↓/j scrolls the panel, not the list (#378)
			v.detailOffset++
			return v
		}
		if v.cursor < len(vis)-1 {
			v.cursor++
		}
	case "up", "k":
		if v.detail {
			if v.detailOffset > 0 {
				v.detailOffset--
			}
			return v
		}
		if v.cursor > 0 {
			v.cursor--
		}
	case "enter":
		if len(vis) > 0 {
			v.detail = true
			v.detailOffset = 0
		}
	case "esc":
		if v.detail {
			v.detail = false
		} else {
			v.filter = ""
		}
	}
	v.selectedID = idAt(vis, v.cursor) // pin the selection to a session
	return v
}

// handleFilterInput edits the filter buffer from a key while the filter line is
// active, resetting the cursor as the filtered list changes.
func (v sessionsView) handleFilterInput(msg tea.KeyMsg) sessionsView {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEsc:
		v.filtering = false
	case tea.KeyBackspace:
		if r := []rune(v.filter); len(r) > 0 {
			v.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		v.filter += string(msg.Runes)
	}
	v.cursor = 0
	return v
}

// filterLine shows the active filter (with a caret while typing).
func (v sessionsView) filterLine() string {
	s := labelStyle.Render("filter ") + v.filter
	if v.filtering {
		s += "▌"
	}
	return s
}
