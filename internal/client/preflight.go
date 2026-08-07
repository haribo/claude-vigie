package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/install"
	"github.com/haribo/claude-vigie/internal/version"
)

// preflight verifies, before the TUI enters the alt-screen, that the daemon is
// reachable, the token valid, and the daemon's build matches this client's — a
// version drift fails in confusing ways. Strict, no bypass
// (docs/design/tui-preflight.md, #357).
func preflight(cfg *config.Config) error {
	if err := testConnection(cfg); err != nil {
		return err
	}
	daemon, err := fetchDaemonVersion(cfg)
	if err != nil {
		return fmt.Errorf("fetching daemon version: %w", err)
	}
	local := api.VersionInfo{Version: version.Version, Commit: version.Commit}
	if !versionsMatch(local, daemon) {
		return fmt.Errorf("version drift — this vigie is %s, the daemon is %s; upgrade the older side to match",
			describeVersion(local), describeVersion(daemon))
	}
	return preflightWatcher(cfg)
}

// watcherStaleAfter mirrors the TUI's threshold (internal/tui watcherStaleAfter):
// a local watcher must have reported within this window to count as running.
const watcherStaleAfter = 15 * time.Second

// preflightWatcher requires a fresh, version-matching local watcher when this
// machine has vigie hooks installed — a hooks-only machine with a dead or
// outdated watcher reports stale statuses. A machine without local hooks is a
// pure observer and needs no watcher (#359).
func preflightWatcher(cfg *config.Config) error {
	installed, err := install.HooksInstalled(config.EnvConfigPath())
	if err != nil {
		return fmt.Errorf("checking local hooks: %w", err)
	}
	if !installed {
		return nil // pure observer
	}
	ws, err := fetchWatcherStatus(cfg)
	if err != nil {
		return fmt.Errorf("fetching watcher status: %w", err)
	}
	if !heartbeatFresh(ws.Machines[cfg.Machine], clock.Now()) {
		return fmt.Errorf("this machine has vigie hooks but its watcher is not running — start it (`vigie watch`, or restart the vigie-watch service)")
	}
	local := api.VersionInfo{Version: version.Version, Commit: version.Commit}
	if wv := ws.Versions[cfg.Machine]; !versionsMatch(local, wv) {
		return fmt.Errorf("this machine's watcher is %s but the tui is %s — restart the vigie-watch service to match",
			describeVersion(wv), describeVersion(local))
	}
	return nil
}

func heartbeatFresh(seen string, now time.Time) bool {
	if seen == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, seen)
	return err == nil && now.Sub(t) <= watcherStaleAfter
}

func fetchWatcherStatus(cfg *config.Config) (api.WatcherStatus, error) {
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
	var ws api.WatcherStatus
	if err := json.NewDecoder(resp.Body).Decode(&ws); err != nil {
		return api.WatcherStatus{}, fmt.Errorf("decoding watcher status: %w", err)
	}
	return ws, nil
}

// versionsMatch reports whether a client and daemon build are compatible: strict
// version-string equality for releases, and commit equality when either side is a
// dev build — a "dev" == "dev" string match across two commits is a false pass
// (#357).
func versionsMatch(local, daemon api.VersionInfo) bool {
	if local.Version == "dev" || daemon.Version == "dev" {
		return local.Commit == daemon.Commit
	}
	return local.Version == daemon.Version
}

func describeVersion(v api.VersionInfo) string {
	if v.Commit != "" && v.Commit != "none" {
		return fmt.Sprintf("%s (commit %s)", v.Version, v.Commit)
	}
	return v.Version
}

func fetchDaemonVersion(cfg *config.Config) (api.VersionInfo, error) {
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
