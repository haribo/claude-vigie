package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/apiclient"
	"github.com/haribo/claude-vigie/internal/config"
)

// httpClient carries a timeout (http.DefaultClient has none); the per-request
// context bounds each call further.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// fetchSessions retrieves the current sessions from the server.
func fetchSessions(cfg *config.Config) ([]api.SessionView, error) {
	return apiclient.Get[[]api.SessionView](cfg, "/api/sessions", "sessions")
}

// fetchUsage retrieves the current subscription usage from the server.
func fetchUsage(cfg *config.Config) (api.UsageReport, error) {
	return apiclient.Get[api.UsageReport](cfg, "/api/usage", "usage")
}

// fetchWatcher retrieves when the server last received a watch report.
func fetchWatcher(cfg *config.Config) (api.WatcherStatus, error) {
	return apiclient.Get[api.WatcherStatus](cfg, "/api/watcher", "watcher status")
}

// fetchPlatform retrieves the Claude platform status the server polls.
func fetchPlatform(cfg *config.Config) (api.PlatformStatus, error) {
	return apiclient.Get[api.PlatformStatus](cfg, "/api/status", "platform status")
}

// fetchVersion retrieves the daemon's build (#341).
func fetchVersion(cfg *config.Config) (api.VersionInfo, error) {
	return apiclient.Get[api.VersionInfo](cfg, "/api/version", "version")
}

// fetchSettings retrieves the server-wide settings.
func fetchSettings(cfg *config.Config) (api.Settings, error) {
	return apiclient.Get[api.Settings](cfg, "/api/settings", "settings")
}

// fetchStats retrieves the analytics rollups and top sessions.
func fetchStats(cfg *config.Config) (api.StatsResponse, error) {
	return apiclient.Get[api.StatsResponse](cfg, "/api/stats", "stats")
}

// setSessionRetention writes the server-wide session-retention window ("" disables).
func setSessionRetention(cfg *config.Config, v string) error {
	body, err := json.Marshal(api.Settings{SessionRetention: v})
	if err != nil {
		return err
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/settings"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}
