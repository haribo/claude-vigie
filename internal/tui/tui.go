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
	events := make(chan struct{}, 1)
	go subscribeEvents(cfg, events)

	m := model{
		fetch:         func() ([]api.SessionView, error) { return fetchSessions(cfg) },
		fetchUsage:    func() (api.UsageReport, error) { return fetchUsage(cfg) },
		fetchWatcher:  func() (api.WatcherStatus, error) { return fetchWatcher(cfg) },
		fetchSettings: func() (api.Settings, error) { return fetchSettings(cfg) },
		toggleRC:      func(id string, enabled bool) error { return toggleRemoteControl(cfg, id, enabled) },
		setRetention:  func(v string) error { return setSessionRetention(cfg, v) },
		prefs:         loadPrefs(),
		events:        events,
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
