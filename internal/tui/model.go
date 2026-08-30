package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
)

// pollInterval is the fallback refresh. SSE pushes data changes instantly, so
// the poll only refreshes time-derived views (relative SEEN, the ended
// threshold, idle hiding, the watcher's freshness) and covers SSE dropouts.
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
	daemonVersion   api.VersionInfo            // the server's build, fetched once (#341)
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

// now reads the injected clock, falling back to the system clock so a model
// built as a struct literal (e.g. in tests) still works.
func (m model) now() time.Time {
	if m.clock == nil {
		return clock.Now()
	}
	return m.clock()
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
	// The whole payload, not the two maps the model happens to keep: a field added
	// to it must reach the sanitizer and its guard, and picking fields out here is
	// how one would not (#635).
	status api.WatcherStatus
	err    error
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
		return watcherMsg{status: s, err: err}
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
			m.daemonVersion = SanitizeVersion(msg.v)
		}
	case watcherMsg:
		m.markRefresh(srcWatcher, msg.err)
		if msg.err == nil {
			ws := sanitizeWatcherStatus(msg.status)
			m.watcherMachines, m.watcherVersions = ws.Machines, ws.Versions
			m.gotWatcher = true
		}
	case statsMsg:
		m.markRefresh(srcStats, msg.err)
		if msg.err == nil {
			m.stats = sanitizeStats(msg.stats)
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

func (m model) View() string {
	var b strings.Builder

	// No title/clock line: open straight on the tab bar.
	b.WriteString(renderTabBar(m.tab, m.width, m.statePill()))
	b.WriteString("\n")

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
		b.WriteString(renderMachines(m.sessions, m.watcherMachines, m.watcherVersions, m.width, m.now()))
	case tabSettings:
		b.WriteString(m.renderSettings())
	}

	if m.tab != tabSessions {
		b.WriteString("\n" + m.footerBlock())
	}
	return b.String()
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
// the tab-bar chrome above and the rule+footer below — the single source of
// truth for the vertical budget (#378). It measures the rendered chrome rather
// than hard-coding line counts, and returns 0 when the height is unknown, which
// callers read as "render unbounded".
func (m model) bodyHeight() int {
	if m.height <= 0 {
		return 0
	}
	chrome := lineCount(renderTabBar(m.tab, m.width, m.statePill()))
	if m.tab != tabSessions {
		chrome += lineCount(m.footerBlock())
	}
	return m.height - chrome
}
