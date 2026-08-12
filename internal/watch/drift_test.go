package watch

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
)

// TestPostSurfacesDrift covers the client half of #384: the daemon's 409 must be
// recognizable as drift (not just logged as another failed report) and must carry
// the server's message, which names both builds and the remediation.
func TestPostSurfacesDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"watcher build 0.3.0 does not match this daemon 0.5.0"}`))
	}))
	defer srv.Close()

	err := post(&config.Config{ServerURL: srv.URL, Token: "t"}, api.ReportRequest{Event: "watch", SessionID: "s"})
	if err == nil {
		t.Fatal("a refused report should error")
	}
	if !isDrift(err) {
		t.Errorf("isDrift(%v) = false, want true", err)
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("error should carry the daemon's message, got %q", err)
	}
}

// TestNonDriftErrorsAreNotDrift keeps the drift path from swallowing ordinary
// failures — a 500 must stay a plain reporting error, not silence the watcher.
func TestNonDriftErrorsAreNotDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := post(&config.Config{ServerURL: srv.URL, Token: "t"}, api.ReportRequest{Event: "watch", SessionID: "s"})
	if err == nil {
		t.Fatal("a 500 should error")
	}
	if isDrift(err) {
		t.Errorf("isDrift(%v) = true, want false", err)
	}
	var he *httpError
	if !errors.As(err, &he) || he.status != http.StatusInternalServerError {
		t.Errorf("error should carry the status, got %v", err)
	}
}

// TestSuccessIsNotAnError guards the normal path through the new error handling.
func TestSuccessIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := post(&config.Config{ServerURL: srv.URL, Token: "t"}, api.ReportRequest{Event: "watch", SessionID: "s"}); err != nil {
		t.Errorf("accepted report returned %v", err)
	}
}
