package server

import (
	"testing"
	"time"

	"github.com/haribo/claude-fleet/internal/store"
)

func TestReportStaleAndToView(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	if !reportStale("", now) {
		t.Error("empty reported_at should be stale")
	}
	if reportStale(rfc(5*time.Second), now) {
		t.Error("a 5s-old report should be fresh")
	}
	if !reportStale(rfc(5*time.Minute), now) {
		t.Error("a 5m-old report should be stale")
	}

	// A fresh report keeps the stored status; a stale one reads as ended.
	fresh := toView(store.Session{Status: "idle", ReportedAt: rfc(2 * time.Second)}, nil, now)
	if fresh.Status != "idle" {
		t.Errorf("fresh status = %q, want idle", fresh.Status)
	}
	stale := toView(store.Session{Status: "idle", ReportedAt: rfc(time.Hour)}, nil, now)
	if stale.Status != "ended" {
		t.Errorf("stale status = %q, want ended", stale.Status)
	}
}
