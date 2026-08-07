package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
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
