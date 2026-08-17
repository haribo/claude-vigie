package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The key hints used to sit permanently at the bottom of the Sessions tab: eleven
// of them, needing 134 columns, so a narrower terminal got a row that was both
// permanently occupied and incomplete (#487). Eleven reminders are never needed at
// once. One hint remains on screen — `h help` — and the full list lives behind it
// (docs/design/sessions-chrome.md § 5bis, #493).

// helpKey is the key that opens and closes the shortcuts modal, on every tab.
const helpKey = "h"

// helpColumns is the shortcut list for a tab, split into the two columns the
// modal draws. The split is by kind, not by length: moving around on the left,
// shaping and acting on the right.
func helpColumns(t tab) (left, right [][2]string) {
	switch t {
	case tabSettings:
		return [][2]string{{"⇥", "next tab"}, {"↑↓", "select"}, {"esc", "back"}},
			[][2]string{{"space/←→", "change"}, {"r", "refresh"}, {stateKey, "state"}, {"q", "quit"}}
	case tabStats:
		return [][2]string{{"⇥", "next tab"}, {"esc", "back"}},
			[][2]string{{"d/w/m/y/t", "period"}, {"r", "refresh"}, {stateKey, "state"}, {"q", "quit"}}
	case tabMachines:
		return [][2]string{{"⇥", "next tab"}, {"esc", "back"}},
			[][2]string{{"r", "refresh"}, {stateKey, "state"}, {"q", "quit"}}
	default:
		return [][2]string{
				{"⇥", "next tab"}, {"↑↓", "select"}, {"⏎", "detail"},
				{"n", "next attention"}, {"/", "filter"}, {"esc", "back"},
			}, [][2]string{
				{"s", "sort"}, {"S", "reverse sort"}, {"g", "group"},
				{"r", "refresh"}, {stateKey, "state"}, {"q", "quit"},
			}
	}
}

// helpHint is the one hint that stays on screen: the way to everything else.
func helpHint() string {
	return keycapStyle.Render(" "+helpKey+" ") + dimStyle.Render(" help")
}

// renderHelp draws the shortcuts modal. It replaces the tab body rather than
// pushing it, so the row budget never depends on whether it is open.
func renderHelp(t tab, width int) string {
	left, right := helpColumns(t)
	const gap = "   "

	// Both columns are padded to their own widest entry, so the two key groups
	// line up whatever the tab.
	lk, ll := widest(left, 0), widest(left, 1)
	rk := widest(right, 0)

	rows := len(left)
	if len(right) > rows {
		rows = len(right)
	}
	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		line := helpEntry(left, i, lk, ll)
		if r := helpEntry(right, i, rk, 0); strings.TrimSpace(r) != "" {
			line += gap + r
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}

	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cMuted).
		Padding(1, 3).
		Render(body)
	title := headerStyle.Render("Shortcuts")
	closeHint := dimStyle.Render(helpKey + " or esc to close")
	// A terminal narrower than the box gets it clipped rather than wrapped: a
	// wrapped modal would cost rows the table has already been budgeted, and the
	// budget is computed before this is drawn.
	return clampWidth(title+"\n"+box+"\n"+closeHint, width)
}

// helpEntry renders one row of a column, padded so the labels align. An index
// past the end of a shorter column renders as blanks, which keeps the two
// columns' rows in step.
func helpEntry(col [][2]string, i, keyWidth, labelWidth int) string {
	if i >= len(col) {
		return strings.Repeat(" ", keyWidth+2+labelWidth)
	}
	key, label := col[i][0], col[i][1]
	return keycapStyle.Render(" "+pad(key, keyWidth)+" ") + " " + dimStyle.Render(pad(label, labelWidth))
}

// widest is the rune width of the widest field f (0 = key, 1 = label) in col.
func widest(col [][2]string, f int) int {
	n := 0
	for _, e := range col {
		if w := len([]rune(e[f])); w > n {
			n = w
		}
	}
	return n
}
