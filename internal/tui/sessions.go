package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

// sortKey identifies how the sessions table is ordered.
type sortKey int

const (
	sortLastSeen sortKey = iota
	sortTokens
	sortStatus
	sortName
	sortRC
	sortKeyCount
)

var sortNames = map[sortKey]string{
	sortLastSeen: "last seen",
	sortTokens:   "tokens",
	sortStatus:   "status",
	sortName:     "name",
	sortRC:       "rc",
}

// groupBy identifies how sessions are grouped in the table.
type groupBy int

const (
	groupNone groupBy = iota
	groupMachine
	groupProject
	groupByCount
)

var groupNames = map[groupBy]string{
	groupNone:    "off",
	groupMachine: "machine",
	groupProject: "project",
}

// sortKeyByName resolves a persisted sort name back to its key, falling back to
// the default (sortLastSeen) for an unknown value — so the stored preference
// survives an enum reorder and a stale/garbage name degrades safely.
func sortKeyByName(name string) sortKey {
	for k, n := range sortNames {
		if n == name {
			return k
		}
	}
	return sortLastSeen
}

// groupByName resolves a persisted group name back to its key, defaulting to
// groupNone for an unknown value.
func groupByName(name string) groupBy {
	for g, n := range groupNames {
		if n == name {
			return g
		}
	}
	return groupNone
}

// sessionsView is the Sessions tab's private state — the cursor and selection,
// the filter/sort/group shaping, the vertical viewport offsets, and the
// per-session status memory for notifications. Extracted from the model so the
// tab owns its own state rather than sharing one god-object (#379).
type sessionsView struct {
	cursor       int
	selectedID   string // session under the cursor, tracked across reorders
	detail       bool
	detailOffset int // scroll offset of the detail view (#378)
	rowOffset    int // sticky top of the sessions viewport, in body-line space (#378)
	filter       string
	filtering    bool
	sortKey      sortKey
	sortReversed bool
	groupBy      groupBy
	prevStatus   map[string]string // last status per session, for notify transitions (#260)
	prevCall     map[string]bool   // last call state per session, for notify transitions (#389)
	blinkOn      bool              // marker on its visible half-cycle (#389)
	blinkTicking bool              // a blink tick is in flight (never stack two)
}

func (m model) handleSessionsKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "/":
		m.sess.filtering = true
	case "s":
		m.sess.sortKey = (m.sess.sortKey + 1) % sortKeyCount
		return m.saveViewPrefs().scrollToCursor()
	case "S":
		m.sess.sortReversed = !m.sess.sortReversed
		return m.saveViewPrefs().scrollToCursor()
	case "g":
		m.sess.groupBy = (m.sess.groupBy + 1) % groupByCount
		return m.saveViewPrefs().scrollToCursor()
	case "n": // jump to the oldest session waiting on the operator (#261) — pure
		// navigation: looking is done in the session, not acknowledged in vigie (ADR-0007)
		if id := nextAttention(m.sessions); id != "" {
			m.sess.selectedID = id
			m.sess.cursor = m.sess.cursorForSelection(m.visibleSessions())
			m.sess.detail = true
			m.sess.detailOffset = 0
		}
	default:
		return m.handleNavKey(msg)
	}
	return m.scrollToCursor()
}

// saveViewPrefs mirrors the live sort/group choices into prefs and persists them
// so they survive a restart (#237). Best-effort: savePrefs never blocks the UI.
func (m model) saveViewPrefs() model {
	m.prefs.sortKey = m.sess.sortKey
	m.prefs.sortReversed = m.sess.sortReversed
	m.prefs.groupBy = m.sess.groupBy
	savePrefs(m.prefs)
	return m
}

func (m model) handleNavKey(msg tea.KeyMsg) model {
	m.sess = m.sess.handleNav(msg, m.visibleSessions())
	return m.scrollToCursor()
}

// scrollToCursor updates the sticky viewport offset so the cursor stays visible
// within the scroll-off margin, using the same budget the renderer uses (#378).
func (m model) scrollToCursor() model {
	tr, budget := m.sessionsBand(m.bodyHeight())
	if budget <= 0 || tr.selected < 0 {
		m.sess.rowOffset = 0
		return m
	}
	_, _, off, _ := window(len(tr.body), tr.selected, budget, m.sess.rowOffset)
	m.sess.rowOffset = off
	return m
}

func idAt(vis []api.SessionView, i int) string {
	if i >= 0 && i < len(vis) {
		return vis[i].ID
	}
	return ""
}

func (m model) handleFilterKey(msg tea.KeyMsg) model {
	m.sess = m.sess.handleFilterInput(msg)
	return m.scrollToCursor()
}

// visibleSessions is the model's view of the Sessions tab's filtered/sorted list,
// threading the shared session data and prefs into the tab's own selector (#379).
func (m model) visibleSessions() []api.SessionView {
	return m.sess.visible(m.sessions, m.prefs, m.now())
}

// overflowBanner is the warning naming the columns the width auto-drop removed
// (the TUI never scrolls sideways). It is wrapped to width so it never runs past
// the edge and gets cut off on a narrow terminal (#325). Empty when all fit.
func overflowBanner(active []column, width int) string {
	over := overflowColumns(active, width)
	if len(over) == 0 {
		return ""
	}
	names := make([]string, len(over))
	for i, c := range over {
		names[i] = c.header
	}
	word := "column"
	if len(over) > 1 {
		word = "columns"
	}
	msg := fmt.Sprintf("⚠ %d %s hidden — terminal too narrow; widen, or deselect in Settings → Columns: %s",
		len(over), word, strings.Join(names, ", "))
	style := warnStyle
	if width > 0 {
		style = style.Width(width) // word-wrap to the terminal width
	}
	return style.Render(msg)
}

func (m model) viewSessions() string {
	// A failed poll must not cost sight of the fleet: the sessions are still in
	// the model, so keep showing them and say they are not current. Only when
	// there is nothing to fall back on does the error stand alone (#456).
	if m.err != nil && len(m.sessions) == 0 {
		return errStyle.Render("error: " + m.err.Error())
	}
	if len(m.sessions) == 0 {
		return dimStyle.Render("no sessions yet")
	}

	bodyHeight := m.bodyHeight()
	vis := m.visibleSessions()
	cursor := clamp(m.sess.cursor, len(vis))
	if m.sess.detail && len(vis) > 0 {
		return m.viewDetail(vis[cursor], bodyHeight)
	}

	var b strings.Builder
	if m.sess.filtering || m.sess.filter != "" {
		b.WriteString(m.sess.filterLine() + "\n")
	}
	active := activeColumns(m.prefs.columnOrder, m.prefs.columnHidden)
	if banner := overflowBanner(active, m.width); banner != "" {
		b.WriteString(banner + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("no sessions match the filter"))
	} else {
		b.WriteString(m.renderTableBand(m.sessionsBand(bodyHeight)))
	}
	b.WriteString("\n" + rule(m.width) + "\n" + m.bottomBar())
	return b.String()
}

// renderTableBand renders the pinned column header and the scrollable row band:
// the whole body when rowBudget <= 0 (it fits, or the height is unknown), else a
// window around the cursor with a sticky group header and a scroll indicator
// (#378). The returned block carries no trailing newline.
func (m model) renderTableBand(tr tableRows, rowBudget int) string {
	var b strings.Builder
	for _, h := range tr.header { // pinned column header + rule
		b.WriteString(h + "\n")
	}
	if rowBudget <= 0 {
		for i, row := range tr.body {
			b.WriteString(row)
			if i < len(tr.body)-1 {
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	start, end, _, _ := window(len(tr.body), tr.selected, rowBudget, m.sess.rowOffset)
	if start > 0 && m.sess.groupBy != groupNone && tr.groupOf[start] >= 0 {
		b.WriteString(tr.body[tr.groupOf[start]] + "\n") // sticky group header
	}
	for _, row := range tr.body[start:end] {
		b.WriteString(row + "\n")
	}
	b.WriteString(scrollIndicator(start, end, len(tr.body), m.width))
	return b.String()
}

// sessionsBand builds the sessions table and returns the row budget for its
// scrollable band: 0 when the whole body fits (or the height is unknown), so the
// caller renders every row; otherwise the number of body rows that fit once the
// fixed chrome, the scroll indicator, and (when grouped) the sticky group header
// are reserved. It is the single budget calculation shared by rendering and by
// the offset update, so the two can never disagree (#378).
func (m model) sessionsBand(bodyHeight int) (tableRows, int) {
	vis := m.visibleSessions()
	cursor := clamp(m.sess.cursor, len(vis))
	active := activeColumns(m.prefs.columnOrder, m.prefs.columnHidden)
	tr := buildTable(vis, active, m.width, cursor, m.sess.groupBy, sortState{m.sess.sortKey, m.sess.sortReversed}, m.frame())
	if bodyHeight <= 0 {
		return tr, 0
	}
	// Fixed chrome inside the sessions body, measured (never hard-coded): the
	// optional filter and overflow-banner lines, the pinned table header, and the
	// trailing rule + bottom bar.
	fixed := 0
	if m.sess.filtering || m.sess.filter != "" {
		fixed += lineCount(m.sess.filterLine())
	}
	if banner := overflowBanner(active, m.width); banner != "" {
		fixed += lineCount(banner)
	}
	fixed += len(tr.header)
	fixed += lineCount(rule(m.width)) + lineCount(m.bottomBar())

	base := bodyHeight - fixed
	if len(tr.body) <= base {
		return tr, 0 // the whole table fits; no windowing, no indicator
	}
	budget := base - 1 // reserve the scroll indicator line
	if m.sess.groupBy != groupNone {
		budget-- // reserve the sticky group header
	}
	if budget < 1 {
		budget = 1
	}
	return tr, budget
}

// viewDetail renders a session's detail panel, scrolled to fit the height with a
// dim indicator when it overflows (#378).
func (m model) viewDetail(s api.SessionView, bodyHeight int) string {
	detail := renderDetail(s)
	if bodyHeight <= 0 {
		return detail
	}
	lines := strings.Split(strings.TrimRight(detail, "\n"), "\n")
	if len(lines) <= bodyHeight {
		return detail
	}
	budget := bodyHeight - 1 // the indicator takes a line
	if budget < 1 {
		budget = 1
	}
	start := clampInt(m.sess.detailOffset, 0, len(lines)-budget)
	var b strings.Builder
	for _, l := range lines[start : start+budget] {
		b.WriteString(l + "\n")
	}
	b.WriteString(scrollIndicator(start, start+budget, len(lines), m.width))
	return b.String()
}

// scrollIndicator is the dim, right-aligned "rows a–b / n" shown on the pinned
// line below a scrolled band (#378).
func scrollIndicator(start, end, total, width int) string {
	label := dimStyle.Render(fmt.Sprintf("rows %d–%d / %d", start+1, end, total))
	if width <= 0 {
		return label
	}
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(label)
}

// bottomBar is the fixed bottom row: the subscription gauges on the left, the
// view state on the right, under one width budget (#486). It replaces the
// separate summary row, whose left half restated the STATUS column and whose
// right half is what survives here (#492).
func (m model) bottomBar() string {
	right, mark := m.viewState()+dimStyle.Render(" · ")+helpHint(), m.staleMark(srcUsage)
	if m.width <= 0 {
		// No width yet (before the first WindowSizeMsg): nothing to budget.
		return joinLR(usageStrip(m.usage, 0)+mark, right, 0)
	}
	// The 3 columns are joinLR's minimum gap between the two halves.
	avail := m.width - lipgloss.Width(right) - 3 - lipgloss.Width(mark)
	if avail <= 0 {
		// Narrower than the view state itself. Keep it: `hidden N` is the only
		// thing on screen saying the list is filtered, and the gauges are figures
		// the Stats tab carries in full.
		return clampWidth(right, m.width)
	}
	return joinLR(usageStrip(m.usage, avail)+mark, right, m.width)
}

// joinLR places left and right on one line, right-aligned to width when known.
// When both cannot fit, it keeps the primary left side (clamped to width) and
// drops the secondary right side rather than overflowing the terminal (#328).
func joinLR(left, right string, width int) string {
	if width <= 0 {
		return left + "   " + right
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+3 > width {
		return lipgloss.NewStyle().MaxWidth(width).Render(left)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// hiddenCount is how many sessions the default filter is currently hiding.
func (m model) hiddenCount() int {
	now := m.now()
	n := 0
	for _, s := range m.sessions {
		if !m.prefs.visible(s, now) {
			n++
		}
	}
	return n
}

// sortSessions orders sessions by key in its natural (default) direction, or
// reversed. Stable, so equal rows keep their prior order.
func sortSessions(s []api.SessionView, key sortKey, reversed bool) {
	sort.SliceStable(s, func(i, j int) bool {
		if reversed {
			return lessBy(s[j], s[i], key)
		}
		return lessBy(s[i], s[j], key)
	})
}

// lessBy reports whether a sorts before b for key in the natural direction:
// most recent, most tokens, most active status, or A→Z name.
func lessBy(a, b api.SessionView, key sortKey) bool {
	switch key {
	case sortTokens:
		return totalTokens(a) > totalTokens(b)
	case sortStatus:
		// status.Rank is an index: lower is higher in the table.
		if ra, rb := a.Rank, b.Rank; ra != rb {
			return ra < rb
		}
		return a.LastSeenAt > b.LastSeenAt // tie-break: most recent first
	case sortName:
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	case sortRC:
		if a.RemoteControl != b.RemoteControl {
			return a.RemoteControl // rc-active first
		}
		return a.LastSeenAt > b.LastSeenAt
	default: // sortLastSeen
		return a.LastSeenAt > b.LastSeenAt
	}
}

// fuzzyMatch reports whether the runes of pattern appear in order in text
// (case-insensitive subsequence match).
func fuzzyMatch(pattern, text string) bool {
	pr := []rune(strings.ToLower(pattern))
	pi := 0
	for _, tr := range strings.ToLower(text) {
		if pi < len(pr) && tr == pr[pi] {
			pi++
		}
	}
	return pi == len(pr)
}

func sessionHaystack(s api.SessionView) string {
	return s.Name + " " + s.Machine + " " + s.Project + " " + s.GitBranch + " " + s.Status
}
