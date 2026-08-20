package watch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/reachability"
)

// TestBeatRecordsWhetherTheDaemonIsReachable guards #578.
//
// The watcher is what makes the fix free on a normal machine: it beats every 5 s
// and already knows the transport is failing, so it can arm the mark before any
// hook has paid a deadline for it. Without this, the first victim of every window
// is one of the operator's tool calls.
//
// HOME is redirected because the mark is derived from it (ADR-0006) — this
// package has no TestMain, so a test that forgets writes into the operator's own
// state directory (#479).
func TestBeatRecordsWhetherTheDaemonIsReachable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusConflict {
			_, _ = w.Write([]byte(`{"error":"build mismatch"}`))
		}
	}))
	defer srv.Close()

	// A port nobody listens on: the transport fails without waiting.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	down := &config.Config{ServerURL: deadURL, Token: "t", Machine: "m"}
	up := &config.Config{ServerURL: srv.URL, Token: "t", Machine: "m"}

	beat(down, false, false)
	if !reachability.Unreachable(deadURL, clock.Now()) {
		t.Error("a failed heartbeat must mark the daemon unreachable, so no hook pays the deadline for it")
	}

	// An answering daemon clears its own mark, and only its own.
	status = http.StatusNoContent
	beat(up, false, false)
	if reachability.Unreachable(srv.URL, clock.Now()) {
		t.Error("an accepted heartbeat must clear the mark")
	}
	if !reachability.Unreachable(deadURL, clock.Now()) {
		t.Error("beating a reachable daemon cleared another daemon's mark")
	}

	// Drift is an answer. The daemon is reachable and refusing the build, which is
	// a different subject — reports must keep being attempted.
	beat(down, false, false) // re-arm
	status = http.StatusConflict
	beat(up, false, false)
	if reachability.Unreachable(srv.URL, clock.Now()) {
		t.Error("a 409 means the daemon answered — it must not read as unreachable")
	}
}

// TestASessionReportKeepsTheMarkFresh: the mark must not depend on the heartbeat
// alone. During an outage the watcher waits out a deadline per session before it
// beats again, so a machine with enough live sessions can take longer than
// StaleAfter to come back round — the mark would expire and hooks would resume
// paying. Every request refreshes it, which is what makes that impossible (#578).
func TestASessionReportKeepsTheMarkFresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	cfg := &config.Config{ServerURL: deadURL, Token: "t", Machine: "m"}

	if err := post(cfg, api.ReportRequest{SessionID: "s1", Event: "watch"}); err == nil {
		t.Fatal("want an error posting to a dead daemon")
	}
	if !reachability.Unreachable(deadURL, clock.Now()) {
		t.Error("a failed session report must refresh the mark, not only the heartbeat")
	}
}
