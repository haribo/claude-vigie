package tui

import (
	"fmt"
	"path/filepath"
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
	labelStyle       = lipgloss.NewStyle().Foreground(cMuted)
	cursorStyle      = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	tabRuleStyle     = lipgloss.NewStyle().Foreground(cAccent)
	groupHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent2)
	keycapStyle      = lipgloss.NewStyle().Foreground(cText).Background(cSurface)
)

// footer renders the keycap-style key hints for the active tab.
func footer(t tab) string {
	hints := [][2]string{
		{"⇥", "switch"}, {"↑↓", "select"}, {"enter", "detail"},
		{"/", "filter"}, {"s", "sort"}, {"S", "reverse"}, {"g", "group"}, {"a", "all"},
		{"r", "refresh"}, {"q", "quit"},
	}
	switch t {
	case tabSettings:
		hints = [][2]string{
			{"↑↓", "select"}, {"space/←→", "change"}, {"⇥", "switch"}, {"r", "refresh"}, {"q", "quit"},
		}
	case tabStats:
		hints = [][2]string{
			{"d/w/m/y/t", "period"}, {"⇥", "switch"}, {"r", "refresh"}, {"q", "quit"},
		}
	case tabMachines:
		hints = [][2]string{
			{"⇥", "switch"}, {"r", "refresh"}, {"q", "quit"},
		}
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = keycapStyle.Render(" "+h[0]+" ") + dimStyle.Render(" "+h[1])
	}
	return strings.Join(parts, "  ")
}

// renderTabBar renders the top-level tab bar: the labels, then a full-width
// separator whose segment under the active tab is an accent underline.
// Tabs are switched with Tab/Shift+Tab, so labels carry no numbers.
func renderTabBar(active tab, width int) string {
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
	return strings.Join(parts, sep) + "\n" + underline
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
	{"NAME", 22, 0, false, sessionName, func(s api.SessionView) lipgloss.Style { return statusStyle(s.Status) }},
	{"USER", 10, 8, false, func(s api.SessionView) string { return orDash(s.User) }, func(api.SessionView) lipgloss.Style { return userStyle }},
	{"MACHINE", 10, 4, false, func(s api.SessionView) string { return s.Machine }, nil},
	{"DIR", 16, 0, false, func(s api.SessionView) string { return projectName(s.ProjectDir) }, nil},
	{"BRANCH", 16, 7, false, func(s api.SessionView) string { return orDash(s.GitBranch) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"MODEL", 12, 5, false, func(s api.SessionView) string { return orDash(shortModel(s.Model)) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"OUT", 8, 3, true, func(s api.SessionView) string { return humanizeTokens(s.Usage.OutputTokens) }, nil},
	{"TOTAL", 9, 2, true, func(s api.SessionView) string { return humanizeTokens(totalTokens(s)) }, nil},
	{"SEEN", 6, 6, true, func(s api.SessionView) string { return relativeAge(s.LastSeenAt, clock.Now()) }, func(api.SessionView) lipgloss.Style { return dimStyle }},
	{"ACT", 10, 9, false, func(s api.SessionView) string { return activitySpark(s.Samples) }, nil},
	{"RC", 4, 1, false, rcCell, rcStyle},
	{"STATUS", 12, 0, false, statusCell, func(s api.SessionView) lipgloss.Style { return statusStyle(s.Status) }},
	{"DOING", 36, 10, false, activityCell, activityStyle},
}

// activityCell renders the short "doing"/"waiting on" message, or a dash.
func activityCell(s api.SessionView) string {
	if s.Activity == "" {
		return "-"
	}
	return s.Activity
}

// activityStyle colors the DOING cell: amber for waiting (a call to action),
// dim for a working turn's tool call.
func activityStyle(s api.SessionView) lipgloss.Style {
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
func renderTable(sessions []api.SessionView, width, selected int, st sortState) string {
	cols := visibleColumns(width)
	var b strings.Builder
	b.WriteString(renderHeaderRow(cols, st) + "\n")
	b.WriteString(rule(width) + "\n")
	for idx, s := range sessions {
		b.WriteString(renderRow(cols, s, idx == selected, width) + "\n")
	}
	return b.String()
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

func renderRow(cols []column, s api.SessionView, selected bool, termWidth int) string {
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
func renderGroupedTable(sessions []api.SessionView, width, selected int, gb groupBy, st sortState) string {
	if gb == groupNone {
		return renderTable(sessions, width, selected, st)
	}

	subtotal := map[string]int64{}
	count := map[string]int{}
	for _, s := range sessions {
		k := groupKey(s, gb)
		subtotal[k] += totalTokens(s)
		count[k]++
	}

	cols := visibleColumns(width)
	var b strings.Builder
	b.WriteString(renderHeaderRow(cols, st) + "\n")
	b.WriteString(rule(width) + "\n")
	lastKey, first := "", true
	for idx, s := range sessions {
		k := groupKey(s, gb)
		if first || k != lastKey {
			b.WriteString(groupHeaderStyle.Render(fmt.Sprintf("▸ %s  (%d · %s)", orDash(k), count[k], humanizeTokens(subtotal[k]))) + "\n")
			lastKey, first = k, false
		}
		b.WriteString(renderRow(cols, s, idx == selected, width) + "\n")
	}
	return b.String()
}

func groupKey(s api.SessionView, gb groupBy) string {
	if gb == groupProject {
		return projectName(s.ProjectDir)
	}
	return s.Machine
}

// renderDetail renders a full-session detail panel.
func renderDetail(s api.SessionView) string {
	name := s.Title
	if name == "" {
		name = s.ID
	}
	lines := []string{
		detailField("Name", name),
		detailField("Session", s.ID),
		detailField("User", orDash(s.User)),
		detailField("Machine", s.Machine),
		detailField("Directory", s.ProjectDir),
		detailField("Branch", orDash(s.GitBranch)),
		detailField("Model", orDash(s.Model)),
		detailField("Status", statusDetail(s)),
		detailField("Doing", orDash(s.Activity)),
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

// sparkWindow is how many recent polls the activity sparkline keeps.
const sparkWindow = 30

// Braille dot bits (offset from U+2800) for a column filled from the bottom up
// to a height of 0..4. The left column uses dots 7,3,2,1; the right, 8,6,5,4.
var (
	brailleLeft  = [5]byte{0, 0x40, 0x44, 0x46, 0x47}
	brailleRight = [5]byte{0, 0x80, 0xA0, 0xB0, 0xB8}
)

// renderSummary renders the fleet summary strip: status counts, total output
// tokens across the fleet, and an activity sparkline (working sessions over the
// recent polls held in history).
func renderSummary(sessions []api.SessionView, history []int) string {
	counts := map[string]int{}
	var totalOut int64
	rcActive := 0
	for _, s := range sessions {
		counts[s.Status]++
		totalOut += s.Usage.OutputTokens
		if s.RemoteControl {
			rcActive++
		}
	}

	parts := []string{
		statusStyle("working").Render(fmt.Sprintf("● working %d", counts["working"])),
	}
	if n := counts["thinking"]; n > 0 { // a sub-state of active work: shown only when present
		parts = append(parts, statusStyle("thinking").Render(fmt.Sprintf("● thinking %d", n)))
	}
	parts = append(parts,
		statusStyle("waiting").Render(fmt.Sprintf("● waiting %d", counts["waiting"])),
		statusStyle("idle").Render(fmt.Sprintf("● idle %d", counts["idle"])),
	)
	if n := counts["error"]; n > 0 { // an alert: shown only when present
		parts = append(parts, statusStyle("error").Render(fmt.Sprintf("● error %d", n)))
	}
	parts = append(parts, dimStyle.Render(fmt.Sprintf("● ended %d", counts["ended"])))
	line := strings.Join(parts, "   ") + "    " + labelStyle.Render("out ") + humanizeTokens(totalOut) +
		"    " + labelStyle.Render("rc ") + rcOnStyle.Render(fmt.Sprintf("◉ %d", rcActive))
	if s := sparkline(history); s != "" {
		line += "    " + labelStyle.Render("activity ") + s
	}
	return line
}

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

func countByStatus(sessions []api.SessionView, status string) int {
	n := 0
	for _, s := range sessions {
		if s.Status == status {
			n++
		}
	}
	return n
}

// visibleColumns returns the columns that fit in width, dropping the
// highest-drop columns first. Columns with drop == 0 are always kept.
func visibleColumns(width int) []column {
	cols := append([]column(nil), columns...)
	if width <= 0 {
		return cols
	}
	for tableWidth(cols) > width {
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
	case "thinking":
		return lipgloss.NewStyle().Foreground(cAccent2) // violet — reasoning inside a turn
	case "error":
		return lipgloss.NewStyle().Foreground(cRed)
	default:
		return dimStyle // ended / unknown
	}
}

// statusCell renders the ● status for the table, appending the HTTP code for a
// live API error (e.g. "● error 529") so the list tells an outage from throttling.
func statusCell(s api.SessionView) string {
	if s.Status == "error" && s.APIErrorStatus != 0 {
		return fmt.Sprintf("● error %d", s.APIErrorStatus)
	}
	return "● " + s.Status
}

// statusDetail renders the status for the detail panel, spelling out an API
// error (e.g. "error — 529 Overloaded").
func statusDetail(s api.SessionView) string {
	if s.Status == "error" && s.APIErrorStatus != 0 {
		return "error — " + apiErrorLabel(s.APIErrorStatus)
	}
	return s.Status
}

// apiErrorLabel names the common Claude API error codes.
func apiErrorLabel(code int) string {
	switch code {
	case 429:
		return "429 Rate limited"
	case 500:
		return "500 Internal server error"
	case 529:
		return "529 Overloaded"
	default:
		return fmt.Sprintf("%d", code)
	}
}

// sessionName is the conversation title, falling back to the short session id.
func sessionName(s api.SessionView) string {
	if s.Title != "" {
		return s.Title
	}
	return shortID(s.ID)
}

func totalTokens(s api.SessionView) int64 {
	return s.Usage.InputTokens + s.Usage.OutputTokens +
		s.Usage.CacheCreationTokens + s.Usage.CacheReadTokens
}

// humanizeTokens renders a token count compactly (e.g. 1234 -> "1.2k").
func humanizeTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// shortModel drops the "claude-" prefix for a compact model label.
func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

// shortID returns the first 8 characters of a session id.
func shortID(id string) string {
	r := []rune(id)
	if len(r) > 8 {
		return string(r[:8])
	}
	return id
}

// projectName returns the final path segment of a project directory.
func projectName(dir string) string {
	if dir == "" {
		return "-"
	}
	return filepath.Base(dir)
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
