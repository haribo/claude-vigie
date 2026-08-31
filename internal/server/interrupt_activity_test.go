package server

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

// #659. A turn the operator killed with Ctrl-C is meant to read `interrupted` in
// the activity column, so it is distinguishable from one that finished cleanly —
// session-status.md § 2 and the v0.4.0 changelog both say so.
//
// The watcher did its half and the server threw the value away: the #236 rule
// blanks the activity on idle, and only `shell` was excepted. Both halves of the
// parser were tested and green; nothing tested the hop where the marker died, so
// the marker had never reached a screen.
func TestActivityInterruptedSurvivesIdle(t *testing.T) {
	watch := func(sess store.Session, isNew bool, status, detail, ts string) store.Session {
		t.Helper()
		return applyReport(sess, isNew, api.ReportRequest{
			SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
			Status: status, Detail: detail, Timestamp: ts,
		})
	}

	// The operator hits Ctrl-C: the turn is over, so the session is idle, and the
	// watcher marks how it ended.
	sess := watch(store.Session{}, true, "idle", "interrupted", "t1")
	if sess.Detail != "interrupted" {
		t.Fatalf("activity = %q, want interrupted to survive idle — a killed turn reads like a finished one", sess.Detail)
	}

	// It clears with no timer: the next report without it is a session that moved
	// on, exactly as `shell` clears when the shell ends.
	sess = watch(sess, false, "idle", "", "t2")
	if sess.Detail != "" {
		t.Errorf("activity = %q, want cleared once the session moved on", sess.Detail)
	}

	// It is a refinement of idle, not a status of its own, and it never outlives a
	// real turn: a working report replaces it with what the session is doing.
	sess = watch(store.Session{}, true, "idle", "interrupted", "t3")
	sess = watch(sess, false, "working", "Edit render.go", "t4")
	if sess.Detail != "Edit render.go" {
		t.Errorf("activity = %q, want the new turn's message", sess.Detail)
	}
}

// A session that ends carries no activity at all — `interrupted` is not an
// exception to that, only to the idle rule.
func TestActivityInterruptedDoesNotSurviveEnded(t *testing.T) {
	sess := applyReport(store.Session{}, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Status: "ended", Detail: "interrupted", Timestamp: "t1",
	})
	if sess.Detail != "" {
		t.Errorf("activity = %q on an ended session, want empty", sess.Detail)
	}
}

// The marker is only worth keeping if it reaches a screen, and the hop that lost
// it was between the stored row and the clients. Both read `detail_text` from
// the session view (ADR-0011), so this is where "visible" is asserted rather
// than in either client.
func TestInterruptedReachesTheSessionView(t *testing.T) {
	sess := applyReport(store.Session{}, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Status: "idle", Detail: "interrupted", Timestamp: "2026-01-01T00:00:00Z",
	})
	sess.ReportedAt = "2026-01-01T00:00:00Z"

	view := toView(sess, nil, time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), true)
	if view.DetailText != "interrupted" {
		t.Errorf("DetailText = %q, want interrupted — the DETAIL cell is what both clients render", view.DetailText)
	}
}
