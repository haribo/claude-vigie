package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-fleet/internal/api"
)

const pollInterval = 2 * time.Second

// tab identifies the active top-level view.
type tab int

const (
	tabSessions tab = iota
	tabUsage
	tabMachines
	tabSettings
)

var tabNames = []string{"Sessions", "Usage", "Machines", "Settings"}

// settingsCount is the number of editable rows in the Settings tab.
const settingsCount = 2

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

type model struct {
	fetch          func() ([]api.SessionView, error)
	fetchUsage     func() (api.UsageReport, error)
	fetchWatcher   func() (api.WatcherStatus, error)
	toggleRC       func(id string, enabled bool) error
	sessions       []api.SessionView
	usage          api.UsageReport
	watcherSeen    string
	gotWatcher     bool
	err            error
	updatedAt      time.Time
	width          int
	tab            tab
	cursor         int
	detail         bool
	history        []int
	filter         string
	filtering      bool
	sortKey        sortKey
	sortReversed   bool
	groupBy        groupBy
	showAll        bool
	prefs          prefs
	settingsCursor int
	events         <-chan struct{}
}

// watcherStaleAfter is how long the server may go without a watch report before
// the TUI warns that statuses may be stale.
const watcherStaleAfter = 15 * time.Second

// watcherStale reports whether no watch report has reached the server recently
// (so the watcher is likely down and statuses are frozen).
func (m model) watcherStale() bool {
	if m.watcherSeen == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, m.watcherSeen)
	if err != nil {
		return false // unknown format: don't cry wolf
	}
	return time.Since(t) > watcherStaleAfter
}

type sessionsMsg struct {
	sessions []api.SessionView
	err      error
}

type tickMsg struct{}

type usageMsg struct {
	usage api.UsageReport
	err   error
}

type eventMsg struct{}

type watcherMsg struct {
	seen string
	err  error
}

type rcDoneMsg struct{ err error }

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.fetchUsageCmd(), m.watcherCmd(), tickCmd(), m.waitForEventCmd())
}

func (m model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.fetch()
		return sessionsMsg{sessions: s, err: err}
	}
}

func (m model) fetchUsageCmd() tea.Cmd {
	return func() tea.Msg {
		u, err := m.fetchUsage()
		return usageMsg{usage: u, err: err}
	}
}

func (m model) watcherCmd() tea.Cmd {
	if m.fetchWatcher == nil {
		return nil
	}
	return func() tea.Msg {
		s, err := m.fetchWatcher()
		return watcherMsg{seen: s.LastSeen, err: err}
	}
}

func (m model) waitForEventCmd() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		<-m.events
		return eventMsg{}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		return m.handleKey(msg)
	case sessionsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.sessions = msg.sessions
			m.err = nil
			m.updatedAt = time.Now()
			m.history = append(m.history, countByStatus(m.sessions, "working"))
			if len(m.history) > sparkWindow {
				m.history = m.history[len(m.history)-sparkWindow:]
			}
		}
	case usageMsg:
		if msg.err == nil {
			m.usage = msg.usage
		}
	case watcherMsg:
		if msg.err == nil {
			m.watcherSeen = msg.seen
			m.gotWatcher = true
		}
	case rcDoneMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, m.fetchCmd() // reflect the new rc state
	case eventMsg:
		return m, tea.Batch(m.fetchCmd(), m.fetchUsageCmd(), m.waitForEventCmd())
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), m.fetchUsageCmd(), m.watcherCmd(), tickCmd())
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilterKey(msg), nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, m.fetchCmd()
	case "c":
		return m, m.toggleSelectedRC()
	}
	return m.handleViewKey(msg), nil
}

// toggleSelectedRC flips the remote-control flag of the selected session (only
// on the Sessions tab). Returns nil when there is nothing to toggle.
func (m model) toggleSelectedRC() tea.Cmd {
	if m.tab != tabSessions || m.toggleRC == nil {
		return nil
	}
	vis := m.visibleSessions()
	if len(vis) == 0 {
		return nil
	}
	s := vis[clamp(m.cursor, len(vis))]
	id, enabled := s.ID, !s.RemoteControl
	return func() tea.Msg {
		return rcDoneMsg{err: m.toggleRC(id, enabled)}
	}
}

func (m model) handleViewKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "tab":
		m.tab = (m.tab + 1) % tab(len(tabNames))
		return m
	case "shift+tab":
		m.tab = (m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames))
		return m
	}
	if m.tab == tabSettings {
		return m.handleSettingsKey(msg)
	}
	return m.handleSessionsKey(msg)
}

func (m model) handleSessionsKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "/":
		m.filtering = true
	case "s":
		m.sortKey = (m.sortKey + 1) % sortKeyCount
	case "S":
		m.sortReversed = !m.sortReversed
	case "g":
		m.groupBy = (m.groupBy + 1) % groupByCount
	case "a":
		m.showAll = !m.showAll
		m.cursor = 0
	default:
		return m.handleNavKey(msg)
	}
	return m
}

func (m model) handleSettingsKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "down", "j":
		if m.settingsCursor < settingsCount-1 {
			m.settingsCursor++
		}
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case " ", "enter", "right", "l":
		m = m.editSetting(1)
	case "left", "h":
		m = m.editSetting(-1)
	}
	return m
}

// editSetting changes the selected preference and persists it. dir is the cycle
// direction for multi-value settings; booleans just toggle.
func (m model) editSetting(dir int) model {
	switch m.settingsCursor {
	case 0:
		m.prefs.hideEnded = !m.prefs.hideEnded
	case 1:
		m.prefs.idleHideAfter = cyclePreset(m.prefs.idleHideAfter, dir)
	}
	savePrefs(m.prefs)
	return m
}

func (m model) handleNavKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "down", "j":
		if m.cursor < len(m.visibleSessions())-1 {
			m.cursor++
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.visibleSessions()) > 0 {
			m.detail = true
		}
	case "esc":
		if m.detail {
			m.detail = false
		} else {
			m.filter = ""
		}
	}
	return m
}

func (m model) handleFilterKey(msg tea.KeyMsg) model {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEsc:
		m.filtering = false
	case tea.KeyBackspace:
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
	}
	m.cursor = 0 // selection resets as the filtered list changes
	return m
}

// visibleSessions returns the sessions after filtering and sorting. Ended and
// stale sessions are hidden unless showAll is set.
func (m model) visibleSessions() []api.SessionView {
	now := time.Now()
	out := make([]api.SessionView, 0, len(m.sessions))
	for _, s := range m.sessions {
		if !m.showAll && !m.prefs.visible(s, now) {
			continue
		}
		if m.matchesFilter(s) {
			out = append(out, s)
		}
	}
	sortSessions(out, m.sortKey, m.sortReversed)
	if m.groupBy != groupNone {
		gb := m.groupBy
		sort.SliceStable(out, func(i, j int) bool {
			return groupKey(out[i], gb) < groupKey(out[j], gb)
		})
	}
	return out
}

func (m model) View() string {
	var b strings.Builder

	// No title/clock line: open straight on the tab bar.
	b.WriteString(renderTabBar(m.tab))
	b.WriteString("\n")
	if m.gotWatcher && m.watcherStale() {
		b.WriteString(warnStyle.Render("⚠ no watcher reporting — statuses may be stale") + "\n")
	}
	b.WriteString("\n")

	switch m.tab {
	case tabSessions:
		b.WriteString(m.viewSessions())
	case tabUsage:
		b.WriteString(renderUsage(m.usage))
	case tabMachines:
		b.WriteString(dimStyle.Render("Machines — coming soon"))
	case tabSettings:
		b.WriteString(m.renderSettings())
	}

	b.WriteString("\n" + rule(m.width) + "\n" + footer(m.tab))
	return b.String()
}

// renderSettings renders the editable preferences.
func (m model) renderSettings() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("Preferences — saved to ~/.config/claude-fleet/tui.toml") + "\n\n")
	rows := []struct{ label, value string }{
		{"Hide ended sessions", onOffLabel(m.prefs.hideEnded)},
		{"Hide idle after", idleLabel(m.prefs.idleHideAfter)},
	}
	for i, r := range rows {
		gutter := "  "
		if i == m.settingsCursor {
			gutter = cursorStyle.Render("❯ ")
		}
		b.WriteString(gutter + labelStyle.Render(pad(r.label, 24)) + r.value + "\n")
	}
	return b.String()
}

func onOffLabel(on bool) string {
	if on {
		return statusStyle("working").Render("on")
	}
	return dimStyle.Render("off")
}

func idleLabel(d time.Duration) string {
	if d == 0 {
		return dimStyle.Render("off (never)")
	}
	return humanizeDuration(d)
}

func (m model) viewSessions() string {
	if m.err != nil {
		return errStyle.Render("error: " + m.err.Error())
	}
	if len(m.sessions) == 0 {
		return dimStyle.Render("no sessions yet")
	}

	vis := m.visibleSessions()
	cursor := clamp(m.cursor, len(vis))
	if m.detail && len(vis) > 0 {
		return renderDetail(vis[cursor])
	}

	var b strings.Builder
	// Summary strip framed by rules at the top of the body.
	b.WriteString(rule(m.width) + "\n")
	b.WriteString(joinLR(renderSummary(m.sessions, m.history), m.summaryRight(), m.width) + "\n")
	b.WriteString(rule(m.width) + "\n")
	if m.filtering || m.filter != "" {
		b.WriteString(m.filterLine() + "\n")
	}
	b.WriteString("\n")
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("no sessions match the filter"))
	} else {
		b.WriteString(renderGroupedTable(vis, m.width, cursor, m.groupBy, sortState{m.sortKey, m.sortReversed}))
	}
	return b.String()
}

// summaryRight is the right-aligned side of the summary strip: the active sort
// (and group), plus the relative last-update age.
func (m model) summaryRight() string {
	parts := []string{labelStyle.Render("sort ") + sortNames[m.sortKey] + sortArrow(m.sortReversed)}
	if m.groupBy != groupNone {
		parts = append(parts, labelStyle.Render("group ")+groupNames[m.groupBy])
	}
	if m.showAll {
		parts = append(parts, labelStyle.Render("showing ")+"all")
	} else if h := m.hiddenCount(); h > 0 {
		parts = append(parts, labelStyle.Render("hidden ")+strconv.Itoa(h))
	}
	if !m.updatedAt.IsZero() {
		parts = append(parts, labelStyle.Render("updated ")+humanizeDuration(time.Since(m.updatedAt))+" ago")
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// filterLine shows the active filter (with a caret while typing).
func (m model) filterLine() string {
	s := labelStyle.Render("filter ") + m.filter
	if m.filtering {
		s += "▌"
	}
	return s
}

// joinLR places left and right on one line, right-aligned to width when known.
func joinLR(left, right string, width int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if width <= 0 || lw+rw+3 > width {
		return left + "   " + right
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// hiddenCount is how many sessions the default filter is currently hiding.
func (m model) hiddenCount() int {
	if m.showAll {
		return 0
	}
	now := time.Now()
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
		if ra, rb := statusRank(a.Status), statusRank(b.Status); ra != rb {
			return ra > rb
		}
		return a.LastSeenAt > b.LastSeenAt // tie-break: most recent first
	case sortName:
		return strings.ToLower(sessionName(a)) < strings.ToLower(sessionName(b))
	case sortRC:
		if a.RemoteControl != b.RemoteControl {
			return a.RemoteControl // rc-active first
		}
		return a.LastSeenAt > b.LastSeenAt
	default: // sortLastSeen
		return a.LastSeenAt > b.LastSeenAt
	}
}

// statusRank orders statuses by activity: working > waiting > idle > ended.
func statusRank(status string) int {
	switch status {
	case "working":
		return 4
	case "waiting":
		return 3
	case "idle":
		return 2
	case "ended":
		return 1
	default:
		return 0
	}
}

// matchesFilter reports whether s passes the current filter. The special
// filter "rc" isolates remote-controlled sessions; anything else is a
// case-insensitive fuzzy match over the session's fields.
func (m model) matchesFilter(s api.SessionView) bool {
	if m.filter == "" {
		return true
	}
	if strings.EqualFold(m.filter, "rc") {
		return s.RemoteControl
	}
	return fuzzyMatch(m.filter, sessionHaystack(s))
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
	return sessionName(s) + " " + s.Machine + " " + projectName(s.ProjectDir) + " " + s.GitBranch + " " + s.Status
}

func clamp(v, n int) int {
	switch {
	case n <= 0, v < 0:
		return 0
	case v >= n:
		return n - 1
	default:
		return v
	}
}
