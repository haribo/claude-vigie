package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestFetchPlatformStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected a User-Agent header when polling the status endpoint")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":{"url":"https://status.claude.com"},"status":{"indicator":"minor","description":"Minor Service Outage"}}`))
	}))
	defer ts.Close()

	ps, err := fetchPlatformStatus(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetchPlatformStatus: %v", err)
	}
	if ps.Indicator != "minor" {
		t.Errorf("indicator = %q, want minor", ps.Indicator)
	}
	if ps.Description != "Minor Service Outage" {
		t.Errorf("description = %q", ps.Description)
	}
	if ps.URL != "https://status.claude.com" {
		t.Errorf("url = %q", ps.URL)
	}
	if ps.FetchedAt == "" {
		t.Error("fetched_at should be stamped")
	}
}

func TestFetchPlatformStatusHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	if _, err := fetchPlatformStatus(context.Background(), ts.URL); err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}

func TestGetPlatformStatus(t *testing.T) {
	srv := newTestServer(t)

	// Before any poll: an empty snapshot, HTTP 200.
	rec := do(t, srv, http.MethodGet, "/api/status", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	var empty api.PlatformStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Indicator != "" {
		t.Errorf("indicator = %q, want empty before first poll", empty.Indicator)
	}

	// Seed a snapshot as the poller would, then read it back.
	blob, _ := json.Marshal(api.PlatformStatus{Indicator: "major", Description: "Elevated errors"})
	if err := srv.store.SetMeta(context.Background(), platformMetaKey, string(blob)); err != nil {
		t.Fatal(err)
	}
	rec = do(t, srv, http.MethodGet, "/api/status", nil, true)
	var got api.PlatformStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Indicator != "major" || got.Description != "Elevated errors" {
		t.Errorf("status = %+v, want major/Elevated errors", got)
	}
}
