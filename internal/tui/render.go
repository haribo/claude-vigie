package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
)

var (
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	dimStyle         = lipgloss.NewStyle().Foreground(cMuted)
	errStyle         = lipgloss.NewStyle().Foreground(cRed)
	warnStyle        = lipgloss.NewStyle().Bold(true).Foreground(cAmber)
	watchLiveStyle   = lipgloss.NewStyle().Foreground(cGreen) // "● live" watcher indicator (#284)
	labelStyle       = lipgloss.NewStyle().Foreground(cMuted)
	cursorStyle      = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	tabRuleStyle     = lipgloss.NewStyle().Foreground(cAccent)
	groupHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent2)
	keycapStyle      = lipgloss.NewStyle().Foreground(cText).Background(cSurface)
)

// renderTabBar renders the top-level tab bar: the labels, then a full-width
// separator whose segment under the active tab is an accent underline.
// Tabs are switched with Tab/Shift+Tab, so labels carry no numbers.
//
// corner is drawn as the last character of the labels line, right-aligned. It
// carries the connection glyph, which used to sit at the end of the deleted
// summary row: that corner is where the eye checks for state, and it never
// changes width, so the table below never jumps
// (docs/design/sessions-chrome.md § 3, #492).
func renderTabBar(active tab, width int, corner string) string {
	const sep = "   "
	parts := make([]string, len(tabNames))
	start, end, pos := 0, 0, 0
	for i, name := range tabNames {
		w := len([]rune(name))
		if tab(i) == active {
			parts[i] = tabActiveStyle.Render(name)
			start, end = pos, pos+w
		} else {
			parts[i] = dimStyle.Render(name)
		}
		pos += w
		if i < len(tabNames)-1 {
			pos += len(sep)
		}
	}

	total := width
	if total < end {
		total = end
	}
	underline := dimStyle.Render(strings.Repeat("─", start)) +
		tabRuleStyle.Render(strings.Repeat("━", end-start)) +
		dimStyle.Render(strings.Repeat("─", total-end))

	labels := strings.Join(parts, sep)
	if corner != "" && width > 0 {
		lw, cw := lipgloss.Width(labels), lipgloss.Width(corner)
		if lw+cw < width {
			labels += strings.Repeat(" ", width-lw-cw) + corner
		}
	}
	return labels + "\n" + underline
}

type column struct {
	header string
	width  int
	// drop is the priority for hiding on narrow terminals: 0 = always kept,
	// higher = dropped first.
	drop  int
	right bool // right-align (numeric columns)
	cell  func(api.SessionView) string
	// style optionally colors the (padded) cell; nil leaves it uncolored.
	style func(api.SessionView) lipgloss.Style
}

// columns in display order (mockup #91): identity, context, numbers (right-
// aligned), then activity, rc, and the colored `● status`. The short session id
// lives in the detail panel only.
var columns = []column{
	{"NAME", 22, 0, false, func(s api.SessionView) string { return s.Name }, func(s api.SessionView) lipgloss.Style { return statusStyle(s.Status) }},
	{"USER", 10, 8, false, func(s api.SessionView) string { return orDash(s.User) }, func(api.SessionView) lipgloss.Style { return userStyle }},
	{"MACHINE", 10, 4, false, func(s api.SessionView) string { return s.Machine }, nil},
	{"DIR", 16, 0, false, func(s api.SessionView) string { return s.Project }, nil},
	{"BRANCH", 16, 7, false, func(s api.SessionView) string { return orDash(s.GitBranch) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"MODEL", 12, 5, false, func(s api.SessionView) string { return orDash(s.ModelShort) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"EFFORT", 6, 11, false, func(s api.SessionView) string { return orDash(s.Effort) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"CTX", 5, 12, true, contextCell, func(s api.SessionView) lipgloss.Style {
		if !contextKnown(s) {
			return dimStyle
		}
		return lipgloss.NewStyle().Foreground(contextColor(contextPct(s)))
	}},
	{"OUT", 6, 3, true, func(s api.SessionView) string { return humanizeTokens(s.Usage.OutputTokens) }, nil},
	{"TOTAL", 7, 2, true, func(s api.SessionView) string { return humanizeTokens(totalTokens(s)) }, nil},
	{"SEEN", 6, 6, true, func(s api.SessionView) string { return relativeAge(s.LastSeenAt, clock.Now()) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"ACT", 10, 9, false, func(s api.SessionView) string { return activitySpark(s.Samples) }, nil},
	{"RC", 3, 1, false, rcCell, rcStyle},
	{"STATUS", 12, 0, false, statusCell, func(s api.SessionView) lipgloss.Style { return statusStyle(s.Status) }},
	{"MODE", 7, 8, false, func(s api.SessionView) string { return s.ModeLabel }, modeStyle},
	{"DETAIL", 36, 10, false, func(s api.SessionView) string { return s.DetailText }, detailStyle},
}

// detailStyle colors the DETAIL cell: amber for waiting (a call to action),
// dim for a working turn's tool call.
func detailStyle(s api.SessionView) lipgloss.Style {
	if hasCall(s) {
		// The call message is the reason the row is blinking: it carries the row's
		// status color rather than the dim default, so the eye that the marker
		// drew lands on a readable reason (#389).
		return statusStyle(s.Status)
	}
	if s.Status == "waiting" {
		return statusStyle("waiting")
	}
	return dimStyle
}

var (
	rcOnStyle  = lipgloss.NewStyle().Foreground(cGreen)
	rcOffStyle = dimStyle
	userStyle  = lipgloss.NewStyle().Foreground(cAccent2) // violet
)

// rcCell renders the remote-control flag: ◉ when active, ○ when inactive.
func rcCell(s api.SessionView) string {
	if s.RemoteControl {
		return "◉"
	}
	return "○"
}

func rcStyle(s api.SessionView) lipgloss.Style {
	if s.RemoteControl {
		return rcOnStyle
	}
	return rcOffStyle
}

const colSep = "  "

// sortState is the active sort, passed to the header so it can mark the sorted
// column with a direction arrow.
type sortState struct {
	key      sortKey
	reversed bool
}

// sortColumn maps a sort key to the header it annotates with an arrow.
var sortColumn = map[sortKey]string{
	sortLastSeen: "SEEN",
	sortTokens:   "TOTAL",
	sortStatus:   "STATUS",
	sortName:     "NAME",
}

func sortArrow(reversed bool) string {
	if reversed {
		return "▲"
	}
	return "▼"
}

// renderTable renders the sessions, dropping low-priority columns to fit width
// (width <= 0 means unknown: show everything). The row at index selected is
// marked with a cursor (selected < 0 for none).
func renderTable(sessions []api.SessionView, base []column, width, selected int, st sortState) string {
	return buildTable(sessions, base, width, selected, groupNone, st, frame{}).join()
}

func renderHeaderRow(cols []column, st sortState) string {
	arrowCol := sortColumn[st.key]
	headers := make([]string, len(cols))
	for i, c := range cols {
		h := c.header
		if c.header == arrowCol {
			h += sortArrow(st.reversed)
		}
		if c.right {
			headers[i] = padLeft(h, c.width)
		} else {
			headers[i] = pad(h, c.width)
		}
	}
	return "  " + headerStyle.Render(strings.Join(headers, colSep))
}

func renderRow(cols []column, s api.SessionView, selected bool, termWidth int, fr frame) string {
	// When selected, the background is applied to every segment (each cell and
	// separator) individually: applying it once over the whole line fails
	// because the cells' own ANSI reset sequences cut it off.
	bg := lipgloss.NewStyle()
	if selected {
		bg = bg.Background(cSel)
	}
	cells := make([]string, len(cols))
	for i, c := range cols {
		var txt string
		if c.right {
			txt = padLeft(c.cell(s), c.width)
		} else {
			txt = pad(c.cell(s), c.width)
		}
		style := bg
		if c.style != nil {
			style = c.style(s)
			if selected {
				style = style.Background(cSel)
			}
		}
		// The call marker replaces the status dot of a calling session, and only
		// that glyph: the status word beside it stays readable, so the animation
		// destroys no information (ADR-0010, #389). Substituting after padding
		// keeps the cell width identical whichever half-cycle we are on.
		if hasCall(s) && c.key() == "status" {
			txt = replaceLeadingDot(txt, fr.callDot())
		}
		cells[i] = style.Render(txt)
	}
	sep := colSep
	if selected {
		sep = bg.Render(colSep)
	}
	body := strings.Join(cells, sep)
	if !selected {
		return "  " + body
	}
	line := cursorStyle.Render("▎") + bg.Render(" ") + body
	if used := rowWidth(cols); termWidth > used {
		line += bg.Render(strings.Repeat(" ", termWidth-used)) // fill to the edge
	}
	return line
}

// renderGroupedTable renders sessions grouped by gb, with a header and token
// subtotal per group. gb == groupNone falls back to a flat table.
func renderGroupedTable(sessions []api.SessionView, base []column, width, selected int, gb groupBy, st sortState) string {
	return buildTable(sessions, base, width, selected, gb, st, frame{}).join()
}

func groupKey(s api.SessionView, gb groupBy) string {
	if gb == groupProject {
		return s.Project
	}
	return s.Machine
}

// renderDetail renders a full-session detail panel.
func renderDetail(s api.SessionView) string {
	lines := []string{
		// s.Name, not `title || full id`: this panel had a fourth naming rule, and
		// it printed the id twice — once as Name, once as Session (#618).
		detailField("Name", s.Name),
		detailField("Session", s.ID),
		detailField("User", orDash(s.User)),
		detailField("Machine", s.Machine),
		detailField("Directory", s.ProjectDir),
		detailField("Branch", orDash(s.GitBranch)),
		detailField("Model", orDash(s.Model)),
		detailField("Effort", orDash(s.Effort)),
		detailField("Context", contextGauge(s)),
		detailField("Status", s.Status),
		detailField("Mode", s.ModeDetail),
		detailField("Detail", orDash(s.Detail)),
		detailField("Remote control", rcLabel(s.RemoteControl)),
	}
	if s.RemoteURL != "" { // the /rc resume link, only while remote control is on
		lines = append(lines, detailField("Remote URL", s.RemoteURL))
	}
	lines = append(lines,
		detailField("Last tool", orDash(s.LastTool)),
		detailField("Started", orDash(s.StartedAt)),
		detailField("Last seen", orDash(s.LastSeenAt)),
		detailField("Ended", orDash(s.EndedAt)),
		"",
		detailField("Input", humanizeTokens(s.Usage.InputTokens)),
		detailField("Output", humanizeTokens(s.Usage.OutputTokens)),
		detailField("Cache create", humanizeTokens(s.Usage.CacheCreationTokens)),
		detailField("Cache read", humanizeTokens(s.Usage.CacheReadTokens)),
		detailField("Total", humanizeTokens(totalTokens(s))),
	)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	return panel
}

func detailField(label, value string) string {
	return labelStyle.Render(pad(label+":", 16)) + value
}

// rcLabel renders the remote-control flag as a symbol + word for the detail panel.
func rcLabel(on bool) string {
	if on {
		return rcOnStyle.Render("◉ on")
	}
	return rcOffStyle.Render("○ off")
}

// Braille dot bits (offset from U+2800) for a column filled from the bottom up
// to a height of 0..4. The left column uses dots 7,3,2,1; the right, 8,6,5,4.
var (
	brailleLeft  = [5]byte{0, 0x40, 0x44, 0x46, 0x47}
	brailleRight = [5]byte{0, 0x80, 0xA0, 0xB0, 0xB8}
)

// sparkline renders values as a braille graph: two samples per glyph (2 columns
// × 4 dot rows), doubling the horizontal resolution of block runes.
func sparkline(values []int) string {
	if len(values) == 0 {
		return ""
	}
	maxV := 1
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	var b strings.Builder
	for i := 0; i < len(values); i += 2 {
		bits := brailleLeft[sparkHeight(values[i], maxV)]
		if i+1 < len(values) {
			bits |= brailleRight[sparkHeight(values[i+1], maxV)]
		}
		b.WriteRune(rune(0x2800 + int(bits)))
	}
	return b.String()
}

// sparkHeight maps a value onto a 0..4 dot column height.
func sparkHeight(v, maxV int) int {
	h := v * 4 / maxV
	switch {
	case h < 0:
		return 0
	case h > 4:
		return 4
	default:
		return h
	}
}

// activitySpark renders a per-session sparkline of token deltas between samples.
func activitySpark(samples []int64) string {
	if len(samples) < 2 {
		return ""
	}
	deltas := make([]int, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		d := int(samples[i] - samples[i-1])
		if d < 0 {
			d = 0
		}
		deltas = append(deltas, d)
	}
	return sparkline(deltas)
}

// visibleColumns returns the columns that fit in width, dropping the
// highest-drop columns first. Columns with drop == 0 are always kept.
func visibleColumns(base []column, width int) []column {
	cols := append([]column(nil), base...)
	if width <= 0 {
		return cols
	}
	for rowWidth(cols) > width { // rowWidth includes the 2-col left gutter
		idx, maxDrop := -1, 0
		for i, c := range cols {
			if c.drop > maxDrop {
				idx, maxDrop = i, c.drop
			}
		}
		if idx < 0 {
			break // only mandatory columns remain
		}
		cols = append(cols[:idx], cols[idx+1:]...)
	}
	return cols
}

func tableWidth(cols []column) int {
	w := 0
	for i, c := range cols {
		if i > 0 {
			w += len(colSep)
		}
		w += c.width
	}
	return w
}

// clampWidth truncates a rendered line to width (ANSI-aware) so it never spills
// past the terminal edge. width <= 0 (unknown) leaves it unchanged. Use for
// tabular rows that have no column-drop of their own; wrap prose instead (#329).
func clampWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// rowWidth is the table width including the 2-column left gutter.
func rowWidth(cols []column) int {
	return 2 + tableWidth(cols)
}

// rule renders a horizontal separator line of n columns (a sensible default if
// the width is unknown).
func rule(n int) string {
	if n <= 0 {
		n = 80
	}
	return dimStyle.Render(strings.Repeat("─", n))
}

// padLeft truncates or left-pads s to exactly w runes (for right-aligned cells).
func padLeft(s string, w int) string {
	s = truncate(s, w)
	if n := w - len([]rune(s)); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "working":
		return lipgloss.NewStyle().Foreground(cGreen)
	case "waiting":
		return lipgloss.NewStyle().Foreground(cAmber)
	case "idle":
		return lipgloss.NewStyle().Foreground(cBlue)
	case "thinking", "compacting":
		// One family, one color: reasoning and compacting are both an active turn,
		// and a color says how much attention a session needs, not what it is busy
		// with (session-status.md § 1bis, #654).
		return lipgloss.NewStyle().Foreground(cGreen)
	case "error":
		return lipgloss.NewStyle().Foreground(cRed)
	case "stalled":
		return lipgloss.NewStyle().Foreground(cOrange) // a foreground tool hung
	default:
		return dimStyle // ended / unknown
	}
}

// statusCell renders the ● status for the table.
//
// The HTTP code of a live API error used to be appended here ("● error 529"),
// which made `error` the only status carrying a refinement inside its own cell.
// design/session-status.md § 2 sends a signal that annotates a state without
// changing it to DETAIL — `shell`, `interrupted`, a permission prompt — and this
// one now goes there too (#584).
func statusCell(s api.SessionView) string {
	if s.Status == "stale" { // dotted: no fresh signal, state unknown (#285)
		return "◌ " + s.Status
	}
	return "● " + s.Status
}

func totalTokens(s api.SessionView) int64 {
	return s.Usage.InputTokens + s.Usage.OutputTokens +
		s.Usage.CacheCreationTokens + s.Usage.CacheReadTokens
}

// humanizeTokens renders a token count compactly (e.g. 1234 -> "1.2k"). The
// dashboard has a twin, and test/fixtures/format-cases.json is what keeps them
// honest (ADR-0011's fourth family, #619).
//
// The arithmetic is integer on purpose. Dividing into a float and rounding the
// result rounds *twice*: 1150/1000 is 1.14999…, which the multiply by ten pulls
// back up to exactly 11.5, which then rounds to 1.2 — while JavaScript, reading
// the same double, answers 1.1. 4004 of the first three million counts diverged
// that way, all of them ending in 50 (though not every count ending in 50 did:
// only those whose quotient falls just under the half). A float that is never
// formed cannot be rounded twice, and both languages compute the same integer.
//
// One decimal is always kept, `1.0k` and not `1k`: this column is aligned on a
// character grid, and a width that changes with the value breaks the column.
func humanizeTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return oneDecimal(n, 1_000_000) + "M"
	case n >= 1_000:
		return oneDecimal(n, 1_000) + "k"
	default:
		return strconv.FormatInt(n, 10)
	}
}

// oneDecimal renders n/unit to one decimal place, the half rounded away from
// zero. `unit/20` is half of one tenth of a unit — the half being rounded — and
// `unit/10` is a tenth, so the whole thing is one integer division.
//
// The addition overflows within 50000 of int64's ceiling and prints nonsense
// there. Not guarded: a session would have to report nine quintillion tokens, and
// a branch on every render to describe an impossible number is worse than the
// sentence you are reading.
func oneDecimal(n, unit int64) string {
	t := (n + unit/20) / (unit / 10)
	return fmt.Sprintf("%d.%d", t/10, t%10)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// pad truncates or right-pads s to exactly w runes.
func pad(s string, w int) string {
	s = truncate(s, w)
	if n := w - len([]rune(s)); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if maxRunes <= 1 || len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-1]) + "…"
}
