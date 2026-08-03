package server

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/store"
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

	// A fresh report keeps the stored status; a stale one on a watched machine
	// reads as ended.
	fresh := toView(store.Session{Status: "idle", ReportedAt: rfc(2 * time.Second)}, nil, now, true)
	if fresh.Status != "idle" {
		t.Errorf("fresh status = %q, want idle", fresh.Status)
	}
	stale := toView(store.Session{Status: "idle", ReportedAt: rfc(time.Hour)}, nil, now, true)
	if stale.Status != "ended" {
		t.Errorf("stale status = %q, want ended", stale.Status)
	}
}

func TestToViewStaleOnUnwatchedMachine(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour).Format(time.RFC3339)

	// A live session whose last report is an old hook, on a machine with no
	// watcher, must NOT be shown ended — "no news" ≠ "dead" (#285).
	sess := store.Session{Status: "working", Machine: "hooks-only", ReportedAt: old}
	if v := toView(sess, nil, now, false); v.Status != "stale" {
		t.Errorf("unwatched stale session = %q, want stale", v.Status)
	}
	// The same session on a watched machine settles to ended, as before.
	if v := toView(sess, nil, now, true); v.Status != "ended" {
		t.Errorf("watched stale session = %q, want ended", v.Status)
	}
	// A session already confirmed ended stays ended regardless of the watcher.
	if v := toView(store.Session{Status: "ended", ReportedAt: old}, nil, now, false); v.Status != "ended" {
		t.Errorf("confirmed-ended session = %q, want ended", v.Status)
	}
}
