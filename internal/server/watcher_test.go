package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestWatcherVersionRoundTrip is the #356 pipeline: a watch report's version is
// stored per machine and surfaced in GET /api/watcher. The build here (0.3.0)
// does not match the test daemon, so the report is refused (409, #384) — and the
// version must round-trip anyway: that is the visibility guarantee, the operator
// has to see which machine is drifted and to what.
func TestWatcherVersionRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	report, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "minet", Status: "working",
		WatcherVersion: "0.3.0", WatcherCommit: "abc1234",
		Timestamp: "2026-08-07T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", report, true); rec.Code != http.StatusConflict {
		t.Fatalf("drifted report = %d, want 409", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/watcher", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get watcher = %d", rec.Code)
	}
	var ws api.WatcherStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}
	got, ok := ws.Versions["minet"]
	if !ok {
		t.Fatalf("no watcher version for minet: %+v", ws.Versions)
	}
	if got.Version != "0.3.0" || got.Commit != "abc1234" {
		t.Errorf("watcher version = %+v, want 0.3.0/abc1234", got)
	}
}
