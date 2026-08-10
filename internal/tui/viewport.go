package tui

import (
	"fmt"
	"strings"

	"github.com/haribo/claude-vigie/internal/api"
)

// scrollOff is the look-ahead margin (#378): the cursor stops this many rows
// from the band edge while more rows exist that way, so a scrolled table always
// shows context beyond the selection.
const scrollOff = 2

// tableRows is the structured render of the sessions table: the pinned header
// lines and the scrollable body lines, plus where the cursor sits and, per body
// line, the group header that governs it — everything the vertical viewport
// (#378) needs to window in rendered-line space without re-deriving group math.
type tableRows struct {
	header   []string // pinned: column header + rule
	body     []string // group headers and session rows, in visual order
	selected int      // index into body of the cursor's row, or -1
	groupOf  []int    // per body line: index of its group header in body, or -1
}

// buildTable renders the sessions into structured lines. gb == groupNone yields
// a flat body; otherwise group headers interleave the rows. It is the single
// source both the string wrappers (renderTable/renderGroupedTable) and the
// viewport build on.
func buildTable(sessions []api.SessionView, base []column, width, selected int, gb groupBy, st sortState) tableRows {
	cols := visibleColumns(base, width)
	tr := tableRows{header: []string{renderHeaderRow(cols, st), rule(width)}, selected: -1}

	if gb == groupNone {
		for idx, s := range sessions {
			if idx == selected {
				tr.selected = len(tr.body)
			}
			tr.body = append(tr.body, renderRow(cols, s, idx == selected, width))
			tr.groupOf = append(tr.groupOf, -1)
		}
		return tr
	}

	subtotal := map[string]int64{}
	count := map[string]int{}
	for _, s := range sessions {
		k := groupKey(s, gb)
		subtotal[k] += totalTokens(s)
		count[k]++
	}
	lastKey, first, curGroup := "", true, -1
	for idx, s := range sessions {
		k := groupKey(s, gb)
		if first || k != lastKey {
			curGroup = len(tr.body)
			tr.body = append(tr.body, groupHeaderStyle.Render(fmt.Sprintf("▸ %s  (%d · %s)", orDash(k), count[k], humanizeTokens(subtotal[k]))))
			tr.groupOf = append(tr.groupOf, -1)
			lastKey, first = k, false
		}
		if idx == selected {
			tr.selected = len(tr.body)
		}
		tr.body = append(tr.body, renderRow(cols, s, idx == selected, width))
		tr.groupOf = append(tr.groupOf, curGroup)
	}
	return tr
}

// join renders the whole table as a string, header then body, matching the
// pre-viewport output exactly (each line followed by a newline).
func (tr tableRows) join() string {
	var b strings.Builder
	for _, l := range tr.header {
		b.WriteString(l + "\n")
	}
	for _, l := range tr.body {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// window computes the slice [start,end) of n body rows visible in a band of
// `budget` rows, keeping `selected` within the scroll-off margin, using and
// updating the sticky `offset`. It is pure and index-only so it carries its own
// unit tests. When everything fits (n <= budget) it returns the whole range and
// a zero offset; `scrolled` reports whether any row is off-screen.
func window(n, selected, budget, offset int) (start, end, newOffset int, scrolled bool) {
	if budget < 1 {
		budget = 1
	}
	if n <= budget {
		return 0, n, 0, false
	}
	maxOffset := n - budget
	// A scroll-off that cannot exceed half the band, so a tiny band still moves.
	so := scrollOff
	if half := (budget - 1) / 2; so > half {
		so = half
	}
	if selected < offset+so {
		offset = selected - so
	}
	if selected > offset+budget-1-so {
		offset = selected - budget + 1 + so
	}
	offset = clampInt(offset, 0, maxOffset)
	return offset, offset + budget, offset, true
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// lineCount is the number of terminal lines a rendered block occupies: 0 for the
// empty string, else one more than its interior newlines. Used to measure the
// variable chrome so the row budget is derived, never hard-coded (#378).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
