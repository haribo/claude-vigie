package tui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/config"
)

// Since #444 every fetcher is a one-line delegation to apiclient.Get, so the only
// thing left in them that can be wrong is the path string — and a wrong path
// fails at runtime against a live daemon, not at compile time. This records which
// endpoint each one asks for.
//
// It also covers fetchVersion, which sat at 0% because the older TestFetchers
// server has no /api/version case and never called it.
func TestEachFetcherAsksItsOwnEndpoint(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		_, _ = w.Write([]byte(`{}`)) // decodes into every type below, and into []T as a failure we ignore
	}))
	defer srv.Close()
	cfg := &config.Config{ServerURL: srv.URL, Token: "tok"}

	for _, tc := range []struct {
		name string
		call func()
		want string
	}{
		{"sessions", func() { _, _ = fetchSessions(cfg) }, "/api/sessions"},
		{"usage", func() { _, _ = fetchUsage(cfg) }, "/api/usage"},
		{"watcher", func() { _, _ = fetchWatcher(cfg) }, "/api/watcher"},
		{"platform", func() { _, _ = fetchPlatform(cfg) }, "/api/status"},
		{"version", func() { _, _ = fetchVersion(cfg) }, "/api/version"},
		{"settings", func() { _, _ = fetchSettings(cfg) }, "/api/settings"},
		{"stats", func() { _, _ = fetchStats(cfg) }, "/api/stats"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked = ""
			tc.call()
			if asked != tc.want {
				t.Errorf("requested %q, want %q", asked, tc.want)
			}
		})
	}
}

// setSessionRetention is the one write, and the only fetcher still building its
// own request: it must POST, not GET, and reach the settings endpoint.
func TestSetRetentionPostsToSettings(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := setSessionRetention(&config.Config{ServerURL: srv.URL, Token: "tok"}, "48h"); err != nil {
		t.Fatalf("setSessionRetention: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if path != "/api/settings" {
		t.Errorf("path = %q, want /api/settings", path)
	}
}
