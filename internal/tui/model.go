package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
)

// pollInterval is the fallback refresh. SSE pushes data changes instantly, so
// the poll only refreshes time-derived views (relative SEEN, the ended
// threshold, idle hiding, the watcher-absent banner) and covers SSE dropouts.
const pollInterval = 5 * time.Second

// tab identifies the active top-level view.
type tab int

const (
	tabSessions tab = iota
	tabStats
	tabMachines
	tabSettings
)

var tabNames = []string{"Sessions", "Stats", "Machines", "Settings"}

// settingsCount is the number of editable rows in the Settings tab.
const settingsCount = 4

// retentionRow is the index of the server session-retention row in Settings;
// notifyRow toggles desktop notifications (#260).
const (
	retentionRow = 2
	notifyRow    = 3
)

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

type model struct {
	fetch           func() ([]api.SessionView, error)
	fetchUsage      func() (api.UsageReport, error)
	fetchWatcher    func() (api.WatcherStatus, error)
	fetchSettings   func() (api.Settings, error)
	fetchStats      func() (api.StatsResponse, error)
	fetchPlatform   func() (api.PlatformStatus, error)
	setRetention    func(v string) error
	serverURL       string // read-only; set via `vigie init`
	serverRetention time.Duration
	stats           api.StatsResponse
	statsPeriod     period
	sessions        []api.SessionView
	usage           api.UsageReport
	platform        api.PlatformStatus
	watcherSeen     string
	watcherMachines map[string]string // per-machine last watch report, RFC3339 (#284)
	gotWatcher      bool
	err             error
	width           int
	tab             tab
	cursor          int
	selectedID      string // session under the cursor, tracked across reorders
	detail          bool
	history         []int
	filter          string
	filtering       bool
	sortKey         sortKey
	sortReversed    bool
	groupBy         groupBy
	fetchSeq        int // generation of the last issued sessions fetch
	appliedSeq      int // generation of the last applied sessions result
	showAll         bool
	prefs           prefs
	settingsCursor  int
	events          <-chan struct{}
	conn            <-chan bool       // server-connection state pushed by the SSE loop
	sseLive         bool              // is the SSE stream currently connected
	clock           func() time.Time  // injected wall clock; defaults to clock.Now
	prevStatus      map[string]string // last status per session, for notify transitions (#260)
	focused         bool              // terminal has focus → suppress desktop notifications
}

// now reads the injected clock, falling back to the system clock so a model
// built as a struct literal (e.g. in tests) still works.
func (m model) now() time.Time {
	if m.clock == nil {
		return clock.Now()
	}
	return m.clock()
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
	gen      int // fetch generation, to drop stale out-of-order responses
}

type tickMsg struct{}

type usageMsg struct {
	usage api.UsageReport
	err   error
}

type platformMsg struct {
	ps  api.PlatformStatus
	err error
}

type eventMsg struct{}

type connMsg struct{ live bool }

type watcherMsg struct {
	seen     string
	machines map[string]string
	err      error
}

type settingsMsg struct {
	retention string
	err       error
}

type retentionDoneMsg struct{ err error }

type statsMsg struct {
	stats api.StatsResponse
	err   error
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(m.fetchSeq), m.fetchUsageCmd(), m.watcherCmd(),
		m.settingsCmd(), m.statsCmd(), m.fetchPlatformCmd(), tickCmd(),
		m.waitForEventCmd(), m.waitForConnCmd())
}

func (m model) statsCmd() tea.Cmd {
	if m.fetchStats == nil {
		return nil
	}
	return func() tea.Msg {
		s, err := m.fetchStats()
		return statsMsg{stats: s, err: err}
	}
}

func (m model) settingsCmd() tea.Cmd {
	if m.fetchSettings == nil {
		return nil
	}
	return func() tea.Msg {
		s, err := m.fetchSettings()
		return settingsMsg{retention: s.SessionRetention, err: err}
	}
}

func (m model) setRetentionCmd(d time.Duration) tea.Cmd {
	if m.setRetention == nil {
		return nil
	}
	v := ""
	if d > 0 {
		v = d.String()
	}
	return func() tea.Msg { return retentionDoneMsg{err: m.setRetention(v)} }
}

func (m model) fetchCmd(gen int) tea.Cmd {
	return func() tea.Msg {
		s, err := m.fetch()
		return sessionsMsg{sessions: s, err: err, gen: gen}
	}
}

// refreshSessions bumps the fetch generation and returns the fetch command, so
// a later out-of-order response can be dropped as stale.
func (m *model) refreshSessions() tea.Cmd {
	m.fetchSeq++
	return m.fetchCmd(m.fetchSeq)
}

func (m model) fetchUsageCmd() tea.Cmd {
	return func() tea.Msg {
		u, err := m.fetchUsage()
		return usageMsg{usage: u, err: err}
	}
}

func (m model) fetchPlatformCmd() tea.Cmd {
	if m.fetchPlatform == nil {
		return nil
	}
	return func() tea.Msg {
		ps, err := m.fetchPlatform()
		return platformMsg{ps: ps, err: err}
	}
}

func (m model) watcherCmd() tea.Cmd {
	if m.fetchWatcher == nil {
		return nil
	}
	return func() tea.Msg {
		s, err := m.fetchWatcher()
		return watcherMsg{seen: s.LastSeen, machines: s.Machines, err: err}
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

func (m model) waitForConnCmd() tea.Cmd {
	if m.conn == nil {
		return nil
	}
	return func() tea.Msg { return connMsg{live: <-m.conn} }
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.FocusMsg:
		m.focused = true // operator is watching → suppress desktop notifications
	case tea.BlurMsg:
		m.focused = false
	case tea.KeyMsg:
		return m.handleKey(msg)
	case retentionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		return m, m.settingsCmd() // confirm the saved value
	case connMsg:
		m.sseLive = msg.live
		return m, m.waitForConnCmd()
	case eventMsg:
		sc := m.refreshSessions()
		return m, tea.Batch(sc, m.fetchUsageCmd(), m.statsCmd(), m.fetchPlatformCmd(), m.waitForEventCmd())
	case tickMsg:
		sc := m.refreshSessions()
		return m, tea.Batch(sc, m.fetchUsageCmd(), m.watcherCmd(), m.statsCmd(), m.fetchPlatformCmd(), tickCmd())
	default:
		return m.applyDataMsg(msg), nil
	}
	return m, nil
}

// applySessions folds a sessions fetch into the model, dropping stale
// out-of-order responses and keeping the cursor on the same session.
func (m model) applySessions(msg sessionsMsg) model {
	if msg.gen <= m.appliedSeq {
		return m // stale out-of-order response; keep the newer state
	}
	m.appliedSeq = msg.gen
	if msg.err != nil {
		m.err = msg.err
		return m
	}
	m = m.withNotifiedTransitions(msg.sessions) // desktop notify on working→attention (#260)
	m.sessions = msg.sessions
	m.err = nil
	m.history = append(m.history, countByStatus(m.sessions, "working"))
	if len(m.history) > sparkWindow {
		m.history = m.history[len(m.history)-sparkWindow:]
	}
	m.cursor = m.cursorForSelection() // keep the cursor on the same session
	return m
}

// applyDataMsg folds a fetch result into the model (no follow-up command).
func (m model) applyDataMsg(msg tea.Msg) model {
	switch msg := msg.(type) {
	case sessionsMsg:
		return m.applySessions(msg)
	case usageMsg:
		if msg.err == nil {
			m.usage = msg.usage
		}
	case platformMsg:
		if msg.err == nil {
			m.platform = msg.ps
		}
	case watcherMsg:
		if msg.err == nil {
			m.watcherSeen = msg.seen
			m.watcherMachines = msg.machines
			m.gotWatcher = true
		}
	case statsMsg:
		if msg.err == nil {
			m.stats = msg.stats
		}
	case settingsMsg:
		if msg.err == nil {
			m.serverRetention = 0
			if d, err := time.ParseDuration(msg.retention); err == nil {
				m.serverRetention = d
			}
		}
	}
	return m
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilterKey(msg), nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, m.refreshSessions()
	}
	return m.handleViewKey(msg)
}

func (m model) handleViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.tab = (m.tab + 1) % tab(len(tabNames))
		return m, nil
	case "shift+tab":
		m.tab = (m.tab + tab(len(tabNames)) - 1) % tab(len(tabNames))
		return m, nil
	}
	switch m.tab {
	case tabSettings:
		return m.handleSettingsKey(msg)
	case tabStats:
		return m.handleStatsKey(msg), nil
	case tabMachines:
		return m, nil // read-only overview, no interactions
	default:
		return m.handleSessionsKey(msg), nil
	}
}

func (m model) handleStatsKey(msg tea.KeyMsg) model {
	if p, ok := periodFromKey(msg.String()); ok {
		m.statsPeriod = p
	}
	return m
}

func (m model) handleSessionsKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "/":
		m.filtering = true
	case "s":
		m.sortKey = (m.sortKey + 1) % sortKeyCount
		return m.saveViewPrefs()
	case "S":
		m.sortReversed = !m.sortReversed
		return m.saveViewPrefs()
	case "g":
		m.groupBy = (m.groupBy + 1) % groupByCount
		return m.saveViewPrefs()
	case "a":
		m.showAll = !m.showAll
		m.cursor = 0
	case "n": // jump to the oldest session waiting on the operator (#261) — pure
		// navigation: looking is done in the session, not acknowledged in vigie (ADR-0007)
		if id := nextAttention(m.sessions); id != "" {
			m.selectedID = id
			m.cursor = m.cursorForSelection()
			m.detail = true
		}
	default:
		return m.handleNavKey(msg)
	}
	return m
}

// saveViewPrefs mirrors the live sort/group choices into prefs and persists them
// so they survive a restart (#237). Best-effort: savePrefs never blocks the UI.
func (m model) saveViewPrefs() model {
	m.prefs.sortKey = m.sortKey
	m.prefs.sortReversed = m.sortReversed
	m.prefs.groupBy = m.groupBy
	savePrefs(m.prefs)
	return m
}

// totalSettingsRows is the base preference rows plus one row per column in the
// column picker (#308).
func totalSettingsRows() int { return settingsCount + len(columns) }

func (m model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onColumn := m.settingsCursor >= settingsCount // a column-picker row
	switch msg.String() {
	case "down", "j":
		if m.settingsCursor < totalSettingsRows()-1 {
			m.settingsCursor++
		}
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case " ", "enter":
		if onColumn {
			return m.toggleColumnRow(), nil
		}
		return m.editSetting(1)
	case "right", "l":
		if !onColumn {
			return m.editSetting(1)
		}
	case "left", "h":
		if !onColumn {
			return m.editSetting(-1)
		}
	case "[": // reorder a visible column up
		if onColumn {
			return m.moveColumnRow(-1), nil
		}
	case "]": // reorder a visible column down
		if onColumn {
			return m.moveColumnRow(1), nil
		}
	}
	return m, nil
}

// columnAtCursor returns the picker column under the settings cursor.
func (m model) columnAtCursor() column {
	return pickerColumns(m.prefs.columnOrder)[m.settingsCursor-settingsCount]
}

// toggleColumnRow shows/hides the column under the cursor (mandatory ones can't
// be hidden) and persists (#308).
func (m model) toggleColumnRow() model {
	m.prefs.columnOrder = toggleColumn(m.prefs.columnOrder, m.columnAtCursor().key())
	savePrefs(m.prefs)
	return m
}

// moveColumnRow reorders the column under the cursor, keeping the cursor on it.
func (m model) moveColumnRow(dir int) model {
	key := m.columnAtCursor().key()
	m.prefs.columnOrder = moveColumn(m.prefs.columnOrder, key, dir)
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		if c.key() == key {
			m.settingsCursor = settingsCount + i
			break
		}
	}
	savePrefs(m.prefs)
	return m
}

// editSetting changes the selected preference. Local prefs (rows 0-1) are saved
// to tui.toml; the server retention (row 2) is written to the server via a Cmd.
func (m model) editSetting(dir int) (tea.Model, tea.Cmd) {
	switch m.settingsCursor {
	case 0:
		m.prefs.hideEnded = !m.prefs.hideEnded
		savePrefs(m.prefs)
	case 1:
		m.prefs.idleHideAfter = cyclePreset(m.prefs.idleHideAfter, dir)
		savePrefs(m.prefs)
	case retentionRow:
		m.serverRetention = cycleRetention(m.serverRetention, dir)
		return m, m.setRetentionCmd(m.serverRetention)
	case notifyRow:
		m.prefs.notify = !m.prefs.notify
		savePrefs(m.prefs)
	}
	return m, nil
}

func (m model) handleNavKey(msg tea.KeyMsg) model {
	vis := m.visibleSessions()
	switch msg.String() {
	case "down", "j":
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(vis) > 0 {
			m.detail = true
		}
	case "esc":
		if m.detail {
			m.detail = false
		} else {
			m.filter = ""
		}
	}
	m.selectedID = idAt(vis, m.cursor) // pin the selection to a session
	return m
}

// cursorForSelection returns the cursor index that keeps selectedID under the
// cursor after a reorder, clamping if the session is gone or none is pinned.
func (m model) cursorForSelection() int {
	vis := m.visibleSessions()
	if m.selectedID != "" {
		for i, s := range vis {
			if s.ID == m.selectedID {
				return i
			}
		}
	}
	return clamp(m.cursor, len(vis))
}

func idAt(vis []api.SessionView, i int) string {
	if i >= 0 && i < len(vis) {
		return vis[i].ID
	}
	return ""
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
	now := m.now()
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
	b.WriteString(renderTabBar(m.tab, m.width))
	b.WriteString("\n")
	if m.gotWatcher && m.watcherStale() {
		b.WriteString(warnStyle.Render("⚠ no watcher reporting — statuses may be stale") + "\n")
	}

	switch m.tab {
	case tabSessions:
		b.WriteString(m.viewSessions())
	case tabStats:
		b.WriteString(m.renderStats())
	case tabMachines:
		b.WriteString(renderMachines(m.sessions, m.watcherMachines, m.width))
	case tabSettings:
		b.WriteString(m.renderSettings())
	}

	b.WriteString("\n" + rule(m.width) + "\n" + footer(m.tab))
	return b.String()
}

// renderSettings renders the editable preferences: local display prefs (saved
// to tui.toml) and the server-wide retention.
func (m model) renderSettings() string {
	var b strings.Builder

	// Connection is read-only here: it is set per machine by `vigie init`
	// and shared with the watcher/reporter. Editing it from the TUI would only
	// change this process and leave the watcher on the old server. The token is
	// never shown.
	b.WriteString(dimStyle.Render("Connection") + "\n\n")
	b.WriteString("  " + labelStyle.Render(pad("Server", 24)) + m.serverURL +
		dimStyle.Render("   (read-only — set via `vigie init`)") + "\n\n")

	b.WriteString(dimStyle.Render("Preferences") + "\n\n")
	rows := []struct {
		label  string
		value  string
		server bool
	}{
		{"Hide ended sessions", onOffLabel(m.prefs.hideEnded), false},
		{"Hide idle after", idleLabel(m.prefs.idleHideAfter), false},
		{"Session retention", retentionLabel(m.serverRetention), true},
		{"Desktop notifications", onOffLabel(m.prefs.notify), false},
	}
	for i, r := range rows {
		gutter := "  "
		if i == m.settingsCursor {
			gutter = cursorStyle.Render("❯ ")
		}
		line := gutter + labelStyle.Render(pad(r.label, 24)) + r.value
		if r.server {
			line += dimStyle.Render("   (server)")
		}
		b.WriteString(line + "\n")
	}

	// Column picker: every column, visible ones first, toggled with space and
	// reordered with [ ] (#308).
	b.WriteString("\n" + dimStyle.Render("Columns") +
		dimStyle.Render("   (space: show/hide    [ ]: reorder)") + "\n\n")
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		gutter := "  "
		if settingsCount+i == m.settingsCursor {
			gutter = cursorStyle.Render("❯ ")
		}
		box := dimStyle.Render("[ ]")
		if columnVisible(m.prefs.columnOrder, c.key()) {
			box = statusStyle("working").Render("[x]")
		}
		name := c.header
		if mandatoryColumns[c.key()] {
			name += dimStyle.Render("  (required)")
		}
		b.WriteString(gutter + box + " " + name + "\n")
	}
	return b.String()
}

func retentionLabel(d time.Duration) string {
	if d == 0 {
		return dimStyle.Render("off (keep all)")
	}
	return humanizeDuration(d)
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
	// The tab-bar separator frames the summary strip above; a rule below
	// separates it from the table.
	b.WriteString(joinLR(renderSummary(m.sessions, m.history), m.summaryRight(), m.width) + "\n")
	b.WriteString(rule(m.width) + "\n")
	if m.filtering || m.filter != "" {
		b.WriteString(m.filterLine() + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("no sessions match the filter"))
	} else {
		b.WriteString(renderGroupedTable(vis, activeColumns(m.prefs.columnOrder), m.width, cursor, m.groupBy, sortState{m.sortKey, m.sortReversed}))
	}
	b.WriteString(rule(m.width) + "\n" + renderUsageStrip(m.usage) + platformStrip(m.platform))
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
	parts = append(parts, m.connGlyph())
	return strings.Join(parts, dimStyle.Render(" · "))
}

// connGlyph is the permanent server-connection indicator, replacing the old
// "updated Xs ago" (which SSE + the 5s poll pinned near zero): ● live when the
// SSE stream is connected, ○ offline when it is down and the last poll also
// failed, ◍ reconnecting otherwise (the poll is still reaching the server).
func (m model) connGlyph() string {
	switch {
	case m.sseLive:
		return lipgloss.NewStyle().Foreground(cGreen).Render("●")
	case m.err != nil:
		return lipgloss.NewStyle().Foreground(cRed).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(cAmber).Render("◍")
	}
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

// statusRank orders statuses for the status sort: a stalled turn (a hung tool
// needing a look) ranks above everything, then working > waiting > idle > ended.
func statusRank(status string) int {
	switch status {
	case "stalled":
		return 5
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
