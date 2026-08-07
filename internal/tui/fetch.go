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
	"github.com/haribo/claude-vigie/internal/config"
)

// httpClient carries a timeout (http.DefaultClient has none); the per-request
// context bounds each call further.
var httpClient = &http.Client{Timeout: 10 * time.Second}

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

	resp, err := httpClient.Do(req)
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

	resp, err := httpClient.Do(req)
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

	resp, err := httpClient.Do(req)
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

// fetchPlatform retrieves the Claude platform status the server polls.
func fetchPlatform(cfg *config.Config) (api.PlatformStatus, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/status"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.PlatformStatus{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return api.PlatformStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.PlatformStatus{}, fmt.Errorf("server returned %s", resp.Status)
	}
	var ps api.PlatformStatus
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return api.PlatformStatus{}, fmt.Errorf("decoding platform status: %w", err)
	}
	return ps, nil
}

// fetchVersion retrieves the daemon's build (#341).
func fetchVersion(cfg *config.Config) (api.VersionInfo, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/version"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.VersionInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return api.VersionInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.VersionInfo{}, fmt.Errorf("server returned %s", resp.Status)
	}
	var v api.VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return api.VersionInfo{}, fmt.Errorf("decoding version: %w", err)
	}
	return v, nil
}

// fetchSettings retrieves the server-wide settings.
func fetchSettings(cfg *config.Config) (api.Settings, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/settings"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.Settings{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return api.Settings{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.Settings{}, fmt.Errorf("server returned %s", resp.Status)
	}
	var s api.Settings
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return api.Settings{}, fmt.Errorf("decoding settings: %w", err)
	}
	return s, nil
}

// fetchStats retrieves the analytics rollups and top sessions.
func fetchStats(cfg *config.Config) (api.StatsResponse, error) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/stats"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.StatsResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return api.StatsResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.StatsResponse{}, fmt.Errorf("server returned %s", resp.Status)
	}
	var s api.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return api.StatsResponse{}, fmt.Errorf("decoding stats: %w", err)
	}
	return s, nil
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
