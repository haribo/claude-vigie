package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// postWatch sends a watch report declaring the given build and returns the status.
func postWatch(t *testing.T, srv *Server, machine, sessionID, ver, commit string) int {
	t.Helper()
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: sessionID, Machine: machine, Status: "working",
		WatcherVersion: ver, WatcherCommit: commit,
		Timestamp: "2026-08-12T10:00:00Z",
	})
	return do(t, srv, http.MethodPost, "/api/report", body, true).Code
}

func sessionIDs(t *testing.T, srv *Server) []string {
	t.Helper()
	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	ids := make([]string, len(views))
	for i, v := range views {
		ids[i] = v.ID
	}
	return ids
}

// TestDriftedWatcherIsRefused is the #384 gate: a watch report whose build does
// not match the daemon is refused and writes no session state, whatever the drift
// looks like — a different release, or a watcher too old to declare a build at all.
func TestDriftedWatcherIsRefused(t *testing.T) {
	cases := []struct {
		name, ver, commit string
	}{
		{"different release", "0.3.0", "abc1234"},
		{"undeclared build", "", ""},
	}
	for _, c := range cases {
		srv := newTestServer(t)
		if got := postWatch(t, srv, "m", "s-"+c.name, c.ver, c.commit); got != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", c.name, got)
		}
		if ids := sessionIDs(t, srv); len(ids) != 0 {
			t.Errorf("%s: drifted report wrote sessions %v, want none", c.name, ids)
		}
	}
}

// TestMatchingWatcherIsAccepted keeps the gate from swallowing the normal path.
func TestMatchingWatcherIsAccepted(t *testing.T) {
	srv := newTestServer(t)
	if got := postWatch(t, srv, "m", "s1", version.Version, version.Commit); got != http.StatusNoContent {
		t.Fatalf("matching watch report = %d, want 204", got)
	}
	if ids := sessionIDs(t, srv); len(ids) != 1 || ids[0] != "s1" {
		t.Errorf("sessions = %v, want [s1]", ids)
	}
}

// TestHookReportUngatedByVersion guards the deliberate exemption: hook reports
// declare no build and run inside the operator's Claude session, so gating them
// would disturb real work (docs/design/version-consistency.md).
func TestHookReportUngatedByVersion(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(api.ReportRequest{
		Event: "UserPromptSubmit", SessionID: "h1", Machine: "m",
		Timestamp: "2026-08-12T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("hook report = %d, want 204", rec.Code)
	}
	if ids := sessionIDs(t, srv); len(ids) != 1 || ids[0] != "h1" {
		t.Errorf("sessions = %v, want [h1]", ids)
	}
}

// TestDriftedMachineStaysVisible is the visibility guarantee that makes the gate
// usable: a drifted watcher writes no sessions, so the machine would vanish from
// GET /api/watcher if that list were derived from sessions alone — and the
// operator would have no way to learn which machine to upgrade, or to what (#384).
func TestDriftedMachineStaysVisible(t *testing.T) {
	srv := newTestServer(t)
	if got := postWatch(t, srv, "ghost", "s1", "0.3.0", "abc1234"); got != http.StatusConflict {
		t.Fatalf("drifted report = %d, want 409", got)
	}

	rec := do(t, srv, http.MethodGet, "/api/watcher", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get watcher = %d", rec.Code)
	}
	var ws api.WatcherStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Machines["ghost"]; !ok {
		t.Errorf("machine absent from watcher status: %+v", ws.Machines)
	}
	got, ok := ws.Versions["ghost"]
	if !ok {
		t.Fatalf("no watcher version for ghost: %+v", ws.Versions)
	}
	if got.Version != "0.3.0" || got.Commit != "abc1234" {
		t.Errorf("watcher version = %+v, want 0.3.0/abc1234", got)
	}
}
