package client

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/apiclient"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/install"
	"github.com/haribo/claude-vigie/internal/presence"
	"github.com/haribo/claude-vigie/internal/tui"
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
		// The daemon's build is named in an error printed straight to the terminal,
		// before the alt-screen and outside the model that cleans everything else
		// (#635). It arrives over the network like any other string.
		return fmt.Errorf("version drift — this vigie is %s, the daemon is %s; upgrade the older side to match",
			describeVersion(local), describeVersion(tui.SanitizeVersion(daemon)))
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
		// A stale heartbeat is a server round-trip failing, not proof of a dead
		// watcher: cross-check a local liveness signal before blaming it (#371).
		if localWatcherRunning() {
			return fmt.Errorf("this machine's watcher is running but the server has no recent heartbeat from it — the server may be unreachable or the watcher just started; check vigied and connectivity, then retry")
		}
		return fmt.Errorf("this machine has vigie hooks but its watcher is not running — start it (`vigie watch`, or restart the vigie-watch service)")
	}
	return watcherBuildError(ws, cfg.Machine)
}

// watcherBuildError refuses a machine whose watcher runs a different build from
// this binary, naming both. Split out of the preflight so the message can be
// tested without a server: it is printed straight to the terminal, before the
// alt-screen, and it quotes a string the watcher chose for itself — so it is
// cleaned first (#635).
func watcherBuildError(ws api.WatcherStatus, machine string) error {
	local := api.VersionInfo{Version: version.Version, Commit: version.Commit}
	wv := tui.SanitizeVersion(ws.Versions[machine])
	if versionsMatch(local, wv) {
		return nil
	}
	return fmt.Errorf("this machine's watcher is %s but the tui is %s — restart the vigie-watch service to match",
		describeVersion(wv), describeVersion(local))
}

// localWatcherRunning reports whether a watcher process for this binary is alive
// on this machine, via a /proc scan (#371). It is a package var so tests can
// stand in for the /proc lookup. A scan error (off Linux, unreadable /proc) is
// treated as "not detected", so the preflight falls back to the plain
// "watcher not running" message rather than guess.
var localWatcherRunning = func() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	running, err := presence.WatcherRunning(filepath.Base(exe), os.Getpid())
	return err == nil && running
}

func heartbeatFresh(seen string, now time.Time) bool {
	if seen == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, seen)
	return err == nil && now.Sub(t) <= watcherStaleAfter
}

func fetchWatcherStatus(cfg *config.Config) (api.WatcherStatus, error) {
	return apiclient.Get[api.WatcherStatus](cfg, "/api/watcher", "watcher status")
}

// versionsMatch reports whether a client and daemon build are compatible. The
// rule itself lives in internal/version, shared with the daemon's watcher gate so
// the fleet has one implementation, not two that drift (#384).
func versionsMatch(local, daemon api.VersionInfo) bool {
	return version.Match(local.Version, local.Commit, daemon.Version, daemon.Commit)
}

func describeVersion(v api.VersionInfo) string {
	return version.Describe(v.Version, v.Commit)
}

func fetchDaemonVersion(cfg *config.Config) (api.VersionInfo, error) {
	return apiclient.Get[api.VersionInfo](cfg, "/api/version", "version")
}
