package tui

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
)

const pollInterval = 2 * time.Second

// tab identifies the active top-level view.
type tab int

const (
	tabSessions tab = iota
	tabUsage
	tabMachines
)

var tabNames = []string{"Sessions", "Usage", "Machines"}

// sortKey identifies how the sessions table is ordered.
type sortKey int

const (
	sortLastSeen sortKey = iota
	sortTokens
	sortStatus
	sortName
	sortKeyCount
)

var sortNames = map[sortKey]string{
	sortLastSeen: "last seen",
	sortTokens:   "tokens",
	sortStatus:   "status",
	sortName:     "name",
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
	fetch      func() ([]api.SessionView, error)
	fetchUsage func() (api.UsageReport, error)
	sessions   []api.SessionView
	usage      api.UsageReport
	err        error
	updated    string
	width      int
	tab        tab
	cursor     int
	detail     bool
	history    []int
	filter     string
	filtering  bool
	sortKey    sortKey
	groupBy    groupBy
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

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.fetchUsageCmd(), tickCmd())
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
			m.updated = time.Now().Format("15:04:05")
			m.history = append(m.history, countByStatus(m.sessions, "working"))
			if len(m.history) > sparkWindow {
				m.history = m.history[len(m.history)-sparkWindow:]
			}
		}
	case usageMsg:
		if msg.err == nil {
			m.usage = msg.usage
		}
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), m.fetchUsageCmd(), tickCmd())
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
	}
	return m.handleViewKey(msg), nil
}

func (m model) handleViewKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "1":
		m.tab = tabSessions
	case "2":
		m.tab = tabUsage
	case "3":
		m.tab = tabMachines
	case "/":
		m.filtering = true
	case "s":
		m.sortKey = (m.sortKey + 1) % sortKeyCount
	case "g":
		m.groupBy = (m.groupBy + 1) % groupByCount
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

// visibleSessions returns the sessions after filtering and sorting.
func (m model) visibleSessions() []api.SessionView {
	out := make([]api.SessionView, 0, len(m.sessions))
	for _, s := range m.sessions {
		if m.filter == "" || fuzzyMatch(m.filter, sessionHaystack(s)) {
			out = append(out, s)
		}
	}
	sortSessions(out, m.sortKey)
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

	b.WriteString(headerStyle.Render("Claude Fleet"))
	if m.updated != "" {
		b.WriteString(dimStyle.Render("  updated " + m.updated))
	}
	b.WriteString("\n")
	b.WriteString(renderTabBar(m.tab))
	b.WriteString("\n\n")

	switch m.tab {
	case tabSessions:
		b.WriteString(m.viewSessions())
	case tabUsage:
		b.WriteString(renderUsage(m.usage))
	case tabMachines:
		b.WriteString(dimStyle.Render("Machines — coming soon"))
	}

	b.WriteString("\n" + footer())
	return b.String()
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
	b.WriteString(renderSummary(m.sessions, m.history) + "\n")
	b.WriteString(m.renderControls() + "\n\n")
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("no sessions match the filter"))
	} else {
		b.WriteString(renderGroupedTable(vis, m.width, cursor, m.groupBy))
	}
	return b.String()
}

func (m model) renderControls() string {
	s := labelStyle.Render("sort ") + sortNames[m.sortKey]
	if m.groupBy != groupNone {
		s += "    " + labelStyle.Render("group ") + groupNames[m.groupBy]
	}
	switch {
	case m.filtering:
		s += "    " + labelStyle.Render("filter ") + m.filter + "▌"
	case m.filter != "":
		s += "    " + labelStyle.Render("filter ") + m.filter
	}
	return s
}

func sortSessions(s []api.SessionView, key sortKey) {
	sort.SliceStable(s, func(i, j int) bool {
		switch key {
		case sortTokens:
			return totalTokens(s[i]) > totalTokens(s[j])
		case sortStatus:
			return s[i].Status < s[j].Status
		case sortName:
			return strings.ToLower(sessionName(s[i])) < strings.ToLower(sessionName(s[j]))
		default: // sortLastSeen
			return s[i].LastSeenAt > s[j].LastSeenAt
		}
	})
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
