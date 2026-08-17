package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/version"
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
	fetch         func() ([]api.SessionView, error)
	fetchUsage    func() (api.UsageReport, error)
	fetchWatcher  func() (api.WatcherStatus, error)
	fetchSettings func() (api.Settings, error)
	fetchStats    func() (api.StatsResponse, error)
	fetchPlatform func() (api.PlatformStatus, error)
	fetchVersion  func() (api.VersionInfo, error)
	setRetention  func(v string) error

	// refreshFailed records which sources failed their last refresh, so a panel
	// says it is showing figures it could not refresh (#449).
	refreshFailed   map[string]bool
	serverURL       string // read-only; set via `vigie init`
	serverRetention time.Duration
	stats           api.StatsResponse
	stat            statsView // the Stats tab's own state (#379)
	sessions        []api.SessionView
	usage           api.UsageReport
	platform        api.PlatformStatus
	daemonVersion   api.VersionInfo // the server's build, fetched once (#341)
	watcherSeen     string
	watcherMachines map[string]string          // per-machine last watch report, RFC3339 (#284)
	watcherVersions map[string]api.VersionInfo // per-machine watcher build (#356)
	gotWatcher      bool
	err             error
	width           int
	height          int // terminal rows; 0 until the first WindowSizeMsg (#378)
	tab             tab
	sess            sessionsView // the Sessions tab's own state (#379)
	fetchSeq        int          // generation of the last issued sessions fetch
	appliedSeq      int          // generation of the last applied sessions result
	prefs           prefs
	set             settingsView // the Settings tab's own state (#379)
	events          <-chan struct{}
	conn            <-chan bool      // server-connection state pushed by the SSE loop
	sseLive         bool             // is the SSE stream currently connected
	clock           func() time.Time // injected wall clock; defaults to clock.Now
	focus           focusState       // what we know of the terminal focus (#411)
	showHelp        bool             // the shortcuts modal is open (#493)
	showState       bool             // the state modal is open (#494)
	pulseOn         bool             // the state pill is on the second tone of its cycle (#495)
	pulseTicking    bool             // a pulse tick is in flight (never stack two)
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

// settingsView is the Settings tab's own state: the row cursor spanning the
// editable prefs and the column picker below them (#379).
type settingsView struct {
	cursor int
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

type versionMsg struct {
	v   api.VersionInfo
	err error
}

type eventMsg struct{}

type connMsg struct{ live bool }

type watcherMsg struct {
	seen     string
	machines map[string]string
	versions map[string]api.VersionInfo
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
		m.settingsCmd(), m.statsCmd(), m.fetchPlatformCmd(), m.fetchVersionCmd(), tickCmd(),
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

func (m model) fetchVersionCmd() tea.Cmd {
	if m.fetchVersion == nil {
		return nil
	}
	return func() tea.Msg {
		v, err := m.fetchVersion()
		return versionMsg{v: v, err: err}
	}
}

func (m model) watcherCmd() tea.Cmd {
	if m.fetchWatcher == nil {
		return nil
	}
	return func() tea.Msg {
		s, err := m.fetchWatcher()
		return watcherMsg{seen: s.LastSeen, machines: s.Machines, versions: s.Versions, err: err}
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

// frame is the current animation state handed to the renderer (#389).
func (m model) frame() frame {
	return frame{hidden: !m.sess.blinkOn, marker: m.prefs.callMarker}
}

type pulseMsg struct{}

type blinkMsg struct{}

// blinkCmd schedules the next marker half-cycle. It is only ever scheduled while
// a call is on screen: the ambient poll is 5 s and must not be raised to animate.
func blinkCmd() tea.Cmd {
	return tea.Tick(blinkInterval, func(time.Time) tea.Msg { return blinkMsg{} })
}

// pulseCmd schedules the next half-cycle of the state pulse. Like the blink, it
// exists exactly as long as something is animating (#495).
func pulseCmd() tea.Cmd {
	return tea.Tick(pulseInterval, func(time.Time) tea.Msg { return pulseMsg{} })
}

// withPulseTick starts the pulse if the pill is degraded and no tick is in
// flight yet.
func (m model) withPulseTick() (tea.Model, tea.Cmd) {
	if m.pulseTicking || !m.pulsing() {
		return m, nil
	}
	m.pulseTicking = true
	return m, pulseCmd()
}

// withBlinkTick starts the animation when a call appears and nothing is animating
// yet, so the tick exists exactly as long as something blinks (#389).
func (m model) withBlinkTick() (tea.Model, tea.Cmd) {
	if m.sess.blinkTicking || !m.frame().blinking(m.visibleSessions()) {
		return m, nil
	}
	m.sess.blinkTicking = true
	return m, blinkCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.scrollToCursor(), nil
	case tea.FocusMsg:
		m.focus = focusOn // operator is watching → suppress desktop notifications
	case tea.BlurMsg:
		m.focus = focusOff
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
	case pulseMsg:
		// The pill recovered: stop the tick and leave the glyph on its full tone.
		if !m.pulsing() {
			m.pulseTicking, m.pulseOn = false, false
			return m, nil
		}
		m.pulseOn = !m.pulseOn
		return m, pulseCmd()
	case blinkMsg:
		// Stop as soon as nothing is calling: the animation must not outlive its
		// reason, and the marker is left visible so no row keeps a blank dot.
		if !m.frame().blinking(m.visibleSessions()) {
			m.sess.blinkTicking, m.sess.blinkOn = false, false
			return m, nil
		}
		m.sess.blinkOn = !m.sess.blinkOn
		return m, blinkCmd()
	default:
		return m.applyDataMsg(msg).withAnimationTicks()
	}
	return m, nil
}

// withAnimationTicks starts whichever of the two animations now has a reason to
// run. They are independent: a call blinking in the table must not decide whether
// a degraded pill breathes, and neither must the other way round (#495).
func (m model) withAnimationTicks() (tea.Model, tea.Cmd) {
	blinked, blinkCmd := m.withBlinkTick()
	pulsed, pulseCmd := blinked.(model).withPulseTick()
	if blinkCmd == nil {
		return pulsed, pulseCmd
	}
	if pulseCmd == nil {
		return pulsed, blinkCmd
	}
	return pulsed, tea.Batch(blinkCmd, pulseCmd)
}

// applySessions folds a sessions fetch into the model, dropping stale
// out-of-order responses and keeping the cursor on the same session.
func (m model) applySessions(msg sessionsMsg) model {
	if msg.gen <= m.appliedSeq {
		return m // stale out-of-order response; keep the newer state
	}
	m.appliedSeq = msg.gen
	m.markRefresh(srcSessions, msg.err)
	if msg.err != nil {
		m.err = msg.err
		return m
	}
	// Cleaned before anything reads them, including the notification path below:
	// a desktop notification is another program's input (#529).
	clean := sanitizeSessions(msg.sessions)
	m = m.withNotifiedTransitions(clean) // desktop notify on working→attention (#260)
	m.sessions = clean
	m.err = nil
	m.sess.cursor = m.sess.cursorForSelection(m.visibleSessions()) // keep the cursor on the same session
	return m
}

// applyDataMsg folds a fetch result into the model (no follow-up command).
func (m model) applyDataMsg(msg tea.Msg) model {
	switch msg := msg.(type) {
	case sessionsMsg:
		return m.applySessions(msg)
	case usageMsg:
		m.markRefresh(srcUsage, msg.err)
		if msg.err == nil {
			m.usage = msg.usage
		}
	case platformMsg:
		m.markRefresh(srcPlatform, msg.err)
		if msg.err == nil {
			m.platform = msg.ps
		}
	case versionMsg:
		m.markRefresh(srcVersion, msg.err)
		if msg.err == nil {
			m.daemonVersion = msg.v
		}
	case watcherMsg:
		m.markRefresh(srcWatcher, msg.err)
		if msg.err == nil {
			m.watcherSeen = msg.seen
			m.watcherMachines = msg.machines
			m.watcherVersions = msg.versions
			m.gotWatcher = true
		}
	case statsMsg:
		m.markRefresh(srcStats, msg.err)
		if msg.err == nil {
			m.stats = msg.stats
		}
	case settingsMsg:
		m.markRefresh(srcSettings, msg.err)
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
	if m.sess.filtering {
		return m.handleFilterKey(msg), nil
	}
	// The shortcuts modal swallows everything but its own two closing keys and
	// the way out of the program: a key pressed at a list of keys must not also
	// act on the table behind it (#493).
	if m.showHelp || m.showState {
		switch msg.String() {
		case "esc":
			m.showHelp, m.showState = false, false
		case helpKey:
			m.showHelp, m.showState = false, false
		case stateKey:
			m.showHelp, m.showState = false, false
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, m.refreshSessions()
	case helpKey:
		m.showHelp = true
		return m, nil
	case stateKey:
		m.showState = true
		return m, nil
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
	m.stat = m.stat.handleKey(msg)
	return m
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

// totalSettingsRows is the base preference rows plus one row per column in the
// column picker (#308).
func totalSettingsRows() int { return settingsCount + len(columns) }

func (m model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onColumn := m.set.cursor >= settingsCount // a column-picker row
	switch msg.String() {
	case "down", "j":
		if m.set.cursor < totalSettingsRows()-1 {
			m.set.cursor++
		}
	case "up", "k":
		if m.set.cursor > 0 {
			m.set.cursor--
		}
	case " ", "enter":
		if onColumn {
			return m.toggleColumnRow(), nil
		}
		return m.editSetting(1)
	case "right":
		if !onColumn {
			return m.editSetting(1)
		}
	case "left":
		if !onColumn {
			return m.editSetting(-1)
		}
	case "[", "shift+up": // reorder the column up
		if onColumn {
			return m.moveColumnRow(-1), nil
		}
	case "]", "shift+down": // reorder the column down
		if onColumn {
			return m.moveColumnRow(1), nil
		}
	}
	return m, nil
}

// columnAtCursor returns the picker column under the settings cursor.
func (m model) columnAtCursor() column {
	return pickerColumns(m.prefs.columnOrder)[m.set.cursor-settingsCount]
}

// toggleColumnRow shows/hides the column under the cursor (mandatory ones can't
// be hidden) and persists (#308).
func (m model) toggleColumnRow() model {
	m.prefs.columnHidden = toggleColumn(m.prefs.columnHidden, m.columnAtCursor().key())
	savePrefs(m.prefs)
	return m
}

// moveColumnRow reorders the column under the cursor, keeping the cursor on it.
func (m model) moveColumnRow(dir int) model {
	key := m.columnAtCursor().key()
	m.prefs.columnOrder = moveColumn(m.prefs.columnOrder, key, dir)
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		if c.key() == key {
			m.set.cursor = settingsCount + i
			break
		}
	}
	savePrefs(m.prefs)
	return m
}

// editSetting changes the selected preference. Local prefs (rows 0-1) are saved
// to tui.toml; the server retention (row 2) is written to the server via a Cmd.
func (m model) editSetting(dir int) (tea.Model, tea.Cmd) {
	switch m.set.cursor {
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

func (m model) View() string {
	var b strings.Builder

	// No title/clock line: open straight on the tab bar.
	b.WriteString(renderTabBar(m.tab, m.width, m.statePill()))
	b.WriteString("\n")
	if m.gotWatcher && m.watcherStale() {
		b.WriteString(m.watcherWarn() + "\n")
	}

	switch {
	case m.showHelp:
		b.WriteString(renderHelp(m.tab, m.width))
		return b.String()
	case m.showState:
		b.WriteString(renderState(m.stateRows(), m.width))
		return b.String()
	}

	switch m.tab {
	case tabSessions:
		b.WriteString(m.viewSessions())
	case tabStats:
		b.WriteString(m.renderStats())
	case tabMachines:
		b.WriteString(m.staleNote(srcWatcher))
		b.WriteString(renderMachines(m.sessions, m.watcherMachines, m.watcherVersions, m.width))
	case tabSettings:
		b.WriteString(m.renderSettings())
	}

	if m.tab != tabSessions {
		b.WriteString("\n" + m.footerBlock())
	}
	return b.String()
}

// watcherWarn is the stale-watcher banner (shown when gotWatcher && watcherStale).
func (m model) watcherWarn() string {
	return warnStyle.Render("⚠ no watcher reporting — statuses may be stale")
}

// footerBlock is the single-hint row for the tabs that have no bottom bar to
// carry it. Sessions folds the hint into its bar instead (#493).
func (m model) footerBlock() string {
	foot := helpHint()
	if m.width > 0 {
		foot = lipgloss.NewStyle().Width(m.width).Render(foot)
	}
	return foot
}

// bodyHeight is the number of terminal rows available to the tab body, between
// the tab-bar/warn chrome above and the rule+footer below — the single source of
// truth for the vertical budget (#378). It measures the rendered chrome rather
// than hard-coding line counts, and returns 0 when the height is unknown, which
// callers read as "render unbounded".
func (m model) bodyHeight() int {
	if m.height <= 0 {
		return 0
	}
	chrome := lineCount(renderTabBar(m.tab, m.width, m.statePill()))
	if m.gotWatcher && m.watcherStale() {
		chrome += lineCount(m.watcherWarn())
	}
	if m.tab != tabSessions {
		chrome += lineCount(m.footerBlock())
	}
	return m.height - chrome
}

// renderSettings renders the editable preferences: local display prefs (saved
// to tui.toml) and the server-wide retention.
// renderBuild shows the client and daemon build versions side by side, flagging
// a drift — a local `vigie` that has fallen behind the `vigied` it talks to,
// which fails in confusing ways on a fleet (#341). The version number is the
// primary display; commit and build time are a dim secondary detail.
func (m model) renderBuild() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("Build") + "\n\n")

	// The version number is the primary line; commit and build time — long, and
	// secondary — go on their own dim line. Each is clamped so the section never
	// overflows a narrow terminal (#341, #331).
	row := func(label, ver, commit, built string) {
		b.WriteString(clampWidth("  "+labelStyle.Render(pad(label, 24))+ver, m.width) + "\n")
		if commit != "" && commit != "none" {
			detail := "commit " + commit
			if built != "" && built != "unknown" {
				detail += " · built " + built
			}
			b.WriteString(clampWidth("    "+dimStyle.Render(detail), m.width) + "\n")
		}
	}
	row("vigie (this client)", version.Version, version.Commit, version.BuildTime)

	if daemon := m.daemonVersion.Version; daemon == "" {
		b.WriteString(clampWidth("  "+labelStyle.Render(pad("vigied (server)", 24))+
			dimStyle.Render("unknown — not reached yet"), m.width) + "\n")
	} else {
		row("vigied (server)", daemon, m.daemonVersion.Commit, m.daemonVersion.BuildTime)
		if daemon != version.Version {
			b.WriteString(warnStyle.Render("  ⚠ client and daemon versions differ") + "\n")
		}
	}
	return b.String()
}

func (m model) renderSettings() string {
	var b strings.Builder
	b.WriteString(m.staleNote(srcSettings, srcVersion))
	// A preferences file that could not be read is kept, not overwritten — and the
	// operator has to be told, or the TUI silently runs on defaults while their
	// settings sit on disk unused (#480).
	if m.prefs.loadFailed != "" {
		b.WriteString(warnStyle.Render("⚠ "+m.prefs.loadFailed+
			" — running on defaults, and leaving the file untouched") + "\n\n")
	}

	// Connection is read-only here: it is set per machine by `vigie init`
	// and shared with the watcher/reporter. Editing it from the TUI would only
	// change this process and leave the watcher on the old server. The token is
	// never shown.
	b.WriteString(dimStyle.Render("Connection") + "\n\n")
	b.WriteString("  " + labelStyle.Render(pad("Server", 24)) + m.serverURL +
		dimStyle.Render("   (read-only — set via `vigie init`)") + "\n\n")

	b.WriteString(m.renderBuild() + "\n")

	b.WriteString(dimStyle.Render("Preferences") + "\n\n")
	rows := []struct {
		label  string
		value  string
		server bool
	}{
		{"Hide ended sessions", onOffLabel(m.prefs.hideEnded), false},
		{"Hide idle after", idleLabel(m.prefs.idleHideAfter), false},
		{"Session retention", retentionLabel(m.serverRetention), true},
		{"Desktop notifications", notifyAvailability(m.prefs.notify).label(), false},
	}
	for i, r := range rows {
		gutter := "  "
		if i == m.set.cursor {
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
	// Width budget: the terminal is width-bound (no horizontal scroll), so show how
	// much the selected columns cost against the available width, and flag the ones
	// the auto-drop cuts off — the drop is never silent (#317).
	active := activeColumns(m.prefs.columnOrder, m.prefs.columnHidden)
	used, avail := rowWidth(active), m.width // rowWidth includes the 2-col gutter
	over := map[string]bool{}
	for _, c := range overflowColumns(active, avail) {
		over[c.key()] = true
	}
	budget := dimStyle.Render(fmt.Sprintf("   width %d/%d", used, avail))
	if used > avail {
		budget = warnStyle.Render(fmt.Sprintf("   width %d/%d — over by %d", used, avail, used-avail))
	}
	header := dimStyle.Render("Columns") +
		dimStyle.Render("   (space: show/hide    [ ] or shift+↑↓: reorder)") + budget
	if m.width > 0 {
		header = lipgloss.NewStyle().Width(m.width).Render(header) // wrap, don't overflow (#329)
	}
	b.WriteString("\n" + header + "\n\n")
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		gutter := "  "
		if settingsCount+i == m.set.cursor {
			gutter = cursorStyle.Render("❯ ")
		}
		box := dimStyle.Render("[ ]")
		if !columnHidden(m.prefs.columnHidden, c.key()) {
			box = statusStyle("working").Render("[x]")
		}
		label := c.header
		suffix := ""
		if mandatoryColumns[c.key()] {
			suffix = " (required)"
		}
		gap := 18 - len(label) - len(suffix)
		if gap < 1 {
			gap = 1
		}
		cost := dimStyle.Render(fmt.Sprintf("w%d", c.width))
		if over[c.key()] {
			cost = warnStyle.Render(fmt.Sprintf("w%d ⚠ cut off", c.width))
		}
		b.WriteString(gutter + box + " " + label + dimStyle.Render(suffix) + strings.Repeat(" ", gap) + cost + "\n")
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
	b.WriteString(m.staleReason())
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

// viewState is what the table cannot say about itself: the active sort and
// grouping, and how many sessions the current filter is hiding.
//
// `hidden N` is the one element of the deleted summary row that exists nowhere
// else on screen — `a` and `idle_hide_after` filter silently, so without it the
// screen claims three sessions while the fleet has thirty. It is omitted when
// nothing is hidden: a permanent zero is a row that trains the eye to skip the
// place where the exception appears (docs/design/sessions-chrome.md § 2).
func (m model) viewState() string {
	parts := []string{labelStyle.Render("sort ") + sortNames[m.sess.sortKey] + sortArrow(m.sess.sortReversed)}
	if m.sess.groupBy != groupNone {
		parts = append(parts, labelStyle.Render("group ")+groupNames[m.sess.groupBy])
	}
	if h := m.hiddenCount(); h > 0 {
		parts = append(parts, labelStyle.Render("hidden ")+strconv.Itoa(h))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// connGlyph is the permanent server-connection indicator, replacing the old
// "updated Xs ago" (which SSE + the 5s poll pinned near zero): ● live when the
// SSE stream is connected, ○ offline when it is down and the last poll also
// failed, ◍ reconnecting otherwise (the poll is still reaching the server).
func (m model) connGlyph() string {
	switch {
	// `sseLive` is an observation, and one made before a suspend is not evidence
	// of anything now. A failing poll is present-tense proof the server is out of
	// reach, so it outranks a stale "connected": the indicator must not assert the
	// one thing it cannot currently know (#457).
	case m.sseLive && m.err == nil:
		return lipgloss.NewStyle().Foreground(cGreen).Render("●")
	case m.err != nil:
		return lipgloss.NewStyle().Foreground(cRed).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(cAmber).Render("◍")
	}
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
		if ra, rb := statusRank(a.Status), statusRank(b.Status); ra != rb {
			return ra < rb
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

// statusRank orders statuses for the status sort, lower first, from the one list
// that also decides which statuses exist (docs/design/session-list.md § 2.1).
//
// It used to name five of the nine and send the rest to a default of 0, which —
// because this comparison was "higher wins" — sorted `compacting`, `thinking`,
// `error` and `stale` *below* `ended`. A session hitting an API error ranked under
// one that was over (#464).
func statusRank(s string) int { return status.Rank(s) }

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
