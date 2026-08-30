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

// #646. The lease is a right to fetch, not a right to hold. A machine that takes
// it and then fetches nothing — most plainly because it has no local credentials
// — kept renewing it every cycle, and every other machine was denied. The gauges
// stayed empty for the whole fleet, permanently, and an empty gauge reads exactly
// like one nobody has filled yet.
func TestAHolderThatFetchedNothingHandsTheLeaseBack(t *testing.T) {
	srv := newTestServer(t)
	ask := func(holder string, release bool) api.LeaseResponse {
		body, _ := json.Marshal(api.LeaseRequest{Holder: holder, Release: release})
		rec := do(t, srv, http.MethodPost, "/api/usage/lease", body, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", holder, rec.Code)
		}
		var lr api.LeaseResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &lr); err != nil {
			t.Fatal(err)
		}
		return lr
	}

	if !ask("m1", false).Acquired {
		t.Fatal("the first holder must acquire")
	}
	if ask("m2", false).Acquired {
		t.Fatal("a second holder must be denied while the lease is held")
	}
	ask("m1", true) // m1 fetched nothing and says so
	if !ask("m2", false).Acquired {
		t.Error("the lease was handed back and m2 still cannot take it — the fleet's gauges stay empty")
	}

	// And a machine that no longer holds it cannot take it from whoever does.
	ask("m1", true)
	if h, _, err := srv.store.LeaseHolder(t.Context(), srv.now()); err != nil || h != "m2" {
		t.Errorf("holder = %q (err %v), want m2 — a stale release must not steal the lease", h, err)
	}
}
