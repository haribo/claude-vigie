// Package tui renders a live terminal dashboard of fleet sessions, polling the
// server. Client side (Bubble Tea); the daemon does not import it.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/config"
)

// Run starts the terminal dashboard, polling the server described by cfg.
func Run(cfg *config.Config) error {
	m := model{
		fetch: func() ([]api.SessionView, error) { return fetchSessions(cfg) },
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
