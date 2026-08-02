package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
)

func TestFetchers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		enc := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.URL.Path == "/api/sessions":
			enc([]api.SessionView{{ID: "s1", Status: "working"}})
		case r.URL.Path == "/api/usage":
			enc(api.UsageReport{FiveHourPct: 47})
		case r.URL.Path == "/api/watcher":
			enc(api.WatcherStatus{LastSeen: "2026-08-02T10:00:00Z"})
		case r.URL.Path == "/api/status":
			enc(api.PlatformStatus{Indicator: "none"})
		case r.URL.Path == "/api/stats":
			enc(api.StatsResponse{SessionCount: 3})
		case r.URL.Path == "/api/settings" && r.Method == http.MethodGet:
			enc(api.Settings{SessionRetention: "720h"})
		case r.URL.Path == "/api/settings" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	cfg := &config.Config{ServerURL: srv.URL, Token: "tok"}

	if s, err := fetchSessions(cfg); err != nil || len(s) != 1 || s[0].ID != "s1" {
		t.Errorf("fetchSessions = %+v, %v", s, err)
	}
	if u, err := fetchUsage(cfg); err != nil || u.FiveHourPct != 47 {
		t.Errorf("fetchUsage = %+v, %v", u, err)
	}
	if wst, err := fetchWatcher(cfg); err != nil || wst.LastSeen == "" {
		t.Errorf("fetchWatcher = %+v, %v", wst, err)
	}
	if p, err := fetchPlatform(cfg); err != nil || p.Indicator != "none" {
		t.Errorf("fetchPlatform = %+v, %v", p, err)
	}
	if st, err := fetchSettings(cfg); err != nil || st.SessionRetention != "720h" {
		t.Errorf("fetchSettings = %+v, %v", st, err)
	}
	if r, err := fetchStats(cfg); err != nil || r.SessionCount != 3 {
		t.Errorf("fetchStats = %+v, %v", r, err)
	}
	if err := setSessionRetention(cfg, "24h"); err != nil {
		t.Errorf("setSessionRetention: %v", err)
	}

	// A wrong token is a 401 → error.
	bad := &config.Config{ServerURL: srv.URL, Token: "wrong"}
	if _, err := fetchSessions(bad); err == nil {
		t.Error("fetchSessions with a bad token should error")
	}
	// A malformed body is a decode error.
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer junk.Close()
	if _, err := fetchSessions(&config.Config{ServerURL: junk.URL, Token: "x"}); err == nil {
		t.Error("fetchSessions on malformed JSON should error")
	}
}
