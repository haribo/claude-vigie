package tui

import (
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

type model struct {
	fetch    func() ([]api.SessionView, error)
	sessions []api.SessionView
	err      error
	updated  string
	width    int
	tab      tab
}

type sessionsMsg struct {
	sessions []api.SessionView
	err      error
}

type tickMsg struct{}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), tickCmd())
}

func (m model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.fetch()
		return sessionsMsg{sessions: s, err: err}
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
		}
	case tickMsg:
		return m, tea.Batch(m.fetchCmd(), tickCmd())
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, m.fetchCmd()
	case "1":
		m.tab = tabSessions
	case "2":
		m.tab = tabUsage
	case "3":
		m.tab = tabMachines
	}
	return m, nil
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
		b.WriteString(dimStyle.Render("Usage — coming soon"))
	case tabMachines:
		b.WriteString(dimStyle.Render("Machines — coming soon"))
	}

	b.WriteString("\n" + dimStyle.Render("1 sessions · 2 usage · 3 machines · r refresh · q quit"))
	return b.String()
}

func (m model) viewSessions() string {
	switch {
	case m.err != nil:
		return errStyle.Render("error: " + m.err.Error())
	case len(m.sessions) == 0:
		return dimStyle.Render("no sessions yet")
	default:
		return renderTable(m.sessions, m.width)
	}
}
