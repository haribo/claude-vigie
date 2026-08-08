package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/install"
)

func TestVersionsMatch(t *testing.T) {
	cases := []struct {
		name          string
		local, daemon api.VersionInfo
		want          bool
	}{
		{"same release", api.VersionInfo{Version: "0.3.0"}, api.VersionInfo{Version: "0.3.0"}, true},
		{"release drift", api.VersionInfo{Version: "0.3.0"}, api.VersionInfo{Version: "0.2.0"}, false},
		{"dev same commit", api.VersionInfo{Version: "dev", Commit: "abc"}, api.VersionInfo{Version: "dev", Commit: "abc"}, true},
		{"dev diff commit", api.VersionInfo{Version: "dev", Commit: "abc"}, api.VersionInfo{Version: "dev", Commit: "def"}, false},
		{"dev vs release, same commit", api.VersionInfo{Version: "dev", Commit: "abc"}, api.VersionInfo{Version: "0.3.0", Commit: "abc"}, true},
		{"dev vs release, diff commit", api.VersionInfo{Version: "dev", Commit: "abc"}, api.VersionInfo{Version: "0.3.0", Commit: "def"}, false},
	}
	for _, c := range cases {
		if got := versionsMatch(c.local, c.daemon); got != c.want {
			t.Errorf("%s: versionsMatch(%+v, %+v) = %v, want %v", c.name, c.local, c.daemon, got, c.want)
		}
	}
}

// preflightServer stands up a fake daemon: /api/sessions returns sessionsCode,
// /api/version returns ver.
func preflightServer(t *testing.T, sessionsCode int, ver api.VersionInfo) *config.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions":
			w.WriteHeader(sessionsCode)
		case "/api/version":
			_ = json.NewEncoder(w).Encode(ver)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &config.Config{ServerURL: srv.URL, Token: "tok"}
}

// TestPreflight covers the #357 gate: pass on a reachable, matching daemon; fail
// on a bad token, a non-vigie 404, a version mismatch, and an unreachable server.
func TestPreflight(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no local hooks → the watcher check (#359) short-circuits
	t.Setenv("VIGIE_CONFIG", "")
	// Under test, version.Version is "dev" and version.Commit is "none".
	okVer := api.VersionInfo{Version: "dev", Commit: "none"}

	if err := preflight(preflightServer(t, http.StatusOK, okVer)); err != nil {
		t.Errorf("matching daemon should pass, got %v", err)
	}
	if err := preflight(preflightServer(t, http.StatusUnauthorized, okVer)); err == nil {
		t.Error("a bad token should fail preflight")
	}
	if err := preflight(preflightServer(t, http.StatusNotFound, okVer)); err == nil {
		t.Error("a non-vigie 404 should fail preflight")
	}
	if err := preflight(preflightServer(t, http.StatusOK, api.VersionInfo{Version: "9.9.9"})); err == nil {
		t.Error("a version mismatch should fail preflight")
	}
	if err := preflight(&config.Config{ServerURL: "http://127.0.0.1:1", Token: "tok"}); err == nil {
		t.Error("an unreachable server should fail preflight")
	}
}

// TestPreflightWatcher covers the #359 gate: a machine with vigie hooks needs a
// fresh, version-matching local watcher; a machine without hooks starts anyway.
func TestPreflightWatcher(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VIGIE_CONFIG", "")

	watcherSrv := func(ws api.WatcherStatus) *config.Config {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(ws)
		}))
		t.Cleanup(srv.Close)
		return &config.Config{ServerURL: srv.URL, Token: "tok", Machine: "host"}
	}
	fresh := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	stale := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)
	okVer := api.VersionInfo{Version: "dev", Commit: "none"} // matches the test build

	// No local hooks yet → pure observer, passes.
	if err := preflightWatcher(watcherSrv(api.WatcherStatus{})); err != nil {
		t.Errorf("no local hooks should pass, got %v", err)
	}

	// Install hooks → a fresh, matching local watcher is now required.
	if _, err := install.Install([]string{"SessionStart"}, "/opt/vigie", "", 5); err != nil {
		t.Fatal(err)
	}
	freshOK := api.WatcherStatus{Machines: map[string]string{"host": fresh}, Versions: map[string]api.VersionInfo{"host": okVer}}
	if err := preflightWatcher(watcherSrv(freshOK)); err != nil {
		t.Errorf("fresh matching watcher should pass, got %v", err)
	}
	if err := preflightWatcher(watcherSrv(api.WatcherStatus{Machines: map[string]string{"host": stale}, Versions: map[string]api.VersionInfo{"host": okVer}})); err == nil {
		t.Error("a stale watcher heartbeat should fail")
	}
	if err := preflightWatcher(watcherSrv(api.WatcherStatus{Machines: map[string]string{"host": fresh}, Versions: map[string]api.VersionInfo{"host": {Version: "9.9.9"}}})); err == nil {
		t.Error("a version-mismatched watcher should fail")
	}
	if err := preflightWatcher(watcherSrv(api.WatcherStatus{})); err == nil {
		t.Error("hooks present but no watcher heartbeat should fail")
	}
}

// TestPreflightWatcherStaleLocalLiveness covers #371: with hooks installed and a
// stale server heartbeat, the remediation depends on whether a local watcher
// process is actually running — "server has no recent heartbeat" when it is, the
// plain "not running" otherwise.
func TestPreflightWatcherStaleLocalLiveness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VIGIE_CONFIG", "")
	if _, err := install.Install([]string{"SessionStart"}, "/opt/vigie", "", 5); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stale := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)
		_ = json.NewEncoder(w).Encode(api.WatcherStatus{Machines: map[string]string{"host": stale}})
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{ServerURL: srv.URL, Token: "tok", Machine: "host"}

	orig := localWatcherRunning
	t.Cleanup(func() { localWatcherRunning = orig })

	localWatcherRunning = func() bool { return true }
	err := preflightWatcher(cfg)
	if err == nil || !strings.Contains(err.Error(), "no recent heartbeat") {
		t.Errorf("live local watcher + stale heartbeat: got %v, want a 'no recent heartbeat' error", err)
	}

	localWatcherRunning = func() bool { return false }
	err = preflightWatcher(cfg)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("no local watcher + stale heartbeat: got %v, want a 'not running' error", err)
	}
}
