package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

func postHeartbeat(t *testing.T, srv *Server, machine, ver, commit string) int {
	t.Helper()
	body, _ := json.Marshal(api.HeartbeatRequest{
		Machine: machine, WatcherVersion: ver, WatcherCommit: commit,
	})
	return do(t, srv, http.MethodPost, "/api/watcher/heartbeat", body, true).Code
}

func watcherStatus(t *testing.T, srv *Server) api.WatcherStatus {
	t.Helper()
	rec := do(t, srv, http.MethodGet, "/api/watcher", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get watcher = %d", rec.Code)
	}
	var ws api.WatcherStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}
	return ws
}

// TestHeartbeatWithoutAnySession is the #386 regression: a watcher that has no
// session to report is still running, and must not read as watcher-less. Before
// the dedicated heartbeat, liveness was a side effect of session reports, so a
// machine with nothing to report vanished from the fleet view.
func TestHeartbeatWithoutAnySession(t *testing.T) {
	srv := newTestServer(t)
	if got := postHeartbeat(t, srv, "idle-box", version.Version, version.Commit); got != http.StatusNoContent {
		t.Fatalf("heartbeat = %d, want 204", got)
	}

	ws := watcherStatus(t, srv)
	seen, ok := ws.Machines["idle-box"]
	if !ok {
		t.Fatalf("machine absent with no session reported: %+v", ws.Machines)
	}
	if seen == "" {
		t.Error("machine present but with no heartbeat timestamp")
	}
	if ws.LastSeen == "" {
		t.Error("fleet-wide last_seen not recorded by a heartbeat")
	}
	if got := ws.Versions["idle-box"]; got.Version != version.Version {
		t.Errorf("watcher version = %+v, want %s", got, version.Version)
	}
	// Liveness must not have invented a session.
	if ids := sessionIDs(t, srv); len(ids) != 0 {
		t.Errorf("heartbeat created sessions %v, want none", ids)
	}
}

// TestDriftedHeartbeatIsRefusedButRecorded: the answer is what tells a drifted
// watcher it may not report (#384), while the claim is still recorded so the
// machine stays visible — the two must not be conflated.
func TestDriftedHeartbeatIsRefusedButRecorded(t *testing.T) {
	srv := newTestServer(t)
	if got := postHeartbeat(t, srv, "old-box", "0.3.0", "abc1234"); got != http.StatusConflict {
		t.Fatalf("drifted heartbeat = %d, want 409", got)
	}

	ws := watcherStatus(t, srv)
	if seen := ws.Machines["old-box"]; seen == "" {
		t.Errorf("drifted machine not visible: %+v", ws.Machines)
	}
	if got := ws.Versions["old-box"]; got.Version != "0.3.0" || got.Commit != "abc1234" {
		t.Errorf("drifted build = %+v, want 0.3.0/abc1234", got)
	}
}

func TestHeartbeatRequiresMachine(t *testing.T) {
	srv := newTestServer(t)
	if got := postHeartbeat(t, srv, "", version.Version, version.Commit); got != http.StatusBadRequest {
		t.Errorf("heartbeat without machine = %d, want 400", got)
	}
	if rec := do(t, srv, http.MethodPost, "/api/watcher/heartbeat", []byte(`not json`), true); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed heartbeat = %d, want 400", rec.Code)
	}
}
