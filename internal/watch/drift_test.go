package watch

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/version"
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

// TestBeatDrivesDriftState covers #386: the heartbeat's answer is what puts the
// watcher into the drifted state and what takes it out, and a transport failure
// is neither — it must not silence a healthy watcher, nor revive a drifted one.
func TestBeatDrivesDriftState(t *testing.T) {
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/watcher/heartbeat" {
			t.Errorf("heartbeat posted to %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusConflict {
			_, _ = w.Write([]byte(`{"error":"this watcher reports 0.3.0, which does not match this daemon (dev)"}`))
		}
	}))
	defer srv.Close()
	cfg := &config.Config{ServerURL: srv.URL, Token: "t", Machine: "m"}

	// A refused build puts the watcher into drift.
	status = http.StatusConflict
	drifted, failing := beat(cfg, false, false)
	if !drifted || failing {
		t.Errorf("after 409: drifted=%v failing=%v, want true/false", drifted, failing)
	}

	// A transport failure leaves the drift verdict untouched — it is not evidence
	// either way — and only marks the transport as failing.
	status = http.StatusInternalServerError
	drifted, failing = beat(cfg, true, false)
	if !drifted || !failing {
		t.Errorf("after 500 while drifted: drifted=%v failing=%v, want true/true", drifted, failing)
	}
	drifted, failing = beat(cfg, false, false)
	if drifted || !failing {
		t.Errorf("after 500 while healthy: drifted=%v failing=%v, want false/true", drifted, failing)
	}

	// An accepted heartbeat clears both.
	status = http.StatusNoContent
	drifted, failing = beat(cfg, true, true)
	if drifted || failing {
		t.Errorf("after 204: drifted=%v failing=%v, want false/false", drifted, failing)
	}
}

// TestBeatDeclaresMachineAndBuild: the claim must identify the machine and its
// build, or the server cannot record which machine is alive, nor at what version.
func TestBeatDeclaresMachineAndBuild(t *testing.T) {
	var got api.HeartbeatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	beat(&config.Config{ServerURL: srv.URL, Token: "t", Machine: "orion"}, false, false)
	if got.Machine != "orion" {
		t.Errorf("machine = %q, want orion", got.Machine)
	}
	if got.WatcherVersion != version.Version || got.WatcherCommit != version.Commit {
		t.Errorf("build = %s/%s, want %s/%s", got.WatcherVersion, got.WatcherCommit, version.Version, version.Commit)
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
