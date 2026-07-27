package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/config"
)

// fetchSessions retrieves the current sessions from the server.
func fetchSessions(cfg *config.Config) ([]api.SessionView, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/sessions"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	var out []api.SessionView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding sessions: %w", err)
	}
	return out, nil
}

// fetchUsage retrieves the current subscription usage from the server.
func fetchUsage(cfg *config.Config) (api.UsageReport, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/usage"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.UsageReport{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return api.UsageReport{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.UsageReport{}, fmt.Errorf("server returned %s", resp.Status)
	}

	var u api.UsageReport
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return api.UsageReport{}, fmt.Errorf("decoding usage: %w", err)
	}
	return u, nil
}

// fetchWatcher retrieves when the server last received a watch report.
func fetchWatcher(cfg *config.Config) (api.WatcherStatus, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/watcher"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.WatcherStatus{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return api.WatcherStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.WatcherStatus{}, fmt.Errorf("server returned %s", resp.Status)
	}

	var s api.WatcherStatus
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return api.WatcherStatus{}, fmt.Errorf("decoding watcher status: %w", err)
	}
	return s, nil
}
