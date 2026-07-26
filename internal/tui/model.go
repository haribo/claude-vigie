package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
)

const pollInterval = 2 * time.Second

type model struct {
	fetch    func() ([]api.SessionView, error)
	sessions []api.SessionView
	err      error
	updated  string
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
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.fetchCmd()
		}
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

func (m model) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Claude Fleet"))
	if m.updated != "" {
		b.WriteString(dimStyle.Render("  updated " + m.updated))
	}
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(errStyle.Render("error: "+m.err.Error()) + "\n")
	case len(m.sessions) == 0:
		b.WriteString(dimStyle.Render("no sessions yet") + "\n")
	default:
		b.WriteString(renderTable(m.sessions))
	}

	b.WriteString("\n" + dimStyle.Render("q quit · r refresh"))
	return b.String()
}
