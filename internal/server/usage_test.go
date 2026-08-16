package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestUsageLeaseAndSnapshot(t *testing.T) {
	srv := newTestServer(t)

	// First holder acquires the lease.
	body, _ := json.Marshal(api.LeaseRequest{Holder: "watcher-1"})
	rec := do(t, srv, http.MethodPost, "/api/usage/lease", body, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("lease = %d", rec.Code)
	}
	var lr api.LeaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &lr); err != nil {
		t.Fatal(err)
	}
	if !lr.Acquired || lr.ExpiresAt == "" {
		t.Fatalf("first holder should acquire: %+v", lr)
	}

	// A different holder is denied while the lease is valid.
	body2, _ := json.Marshal(api.LeaseRequest{Holder: "watcher-2"})
	rec = do(t, srv, http.MethodPost, "/api/usage/lease", body2, true)
	var lr2 api.LeaseResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &lr2)
	if lr2.Acquired {
		t.Error("second holder should be denied while the lease is held")
	}

	// Post then read the usage snapshot.
	// The holder is checked against the lease acquired above (#515).
	u, _ := json.Marshal(api.UsageReport{FiveHourPct: 2, SevenDayPct: 27, FetchedAt: "2026-07-27T10:00:00Z", Holder: "watcher-1"})
	if rec := do(t, srv, http.MethodPost, "/api/usage", u, true); rec.Code != http.StatusNoContent {
		t.Fatalf("post usage = %d", rec.Code)
	}
	rec = do(t, srv, http.MethodGet, "/api/usage", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get usage = %d", rec.Code)
	}
	var got api.UsageReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.FiveHourPct != 2 || got.SevenDayPct != 27 {
		t.Errorf("usage = %+v, want 5h=2 7d=27", got)
	}
}
