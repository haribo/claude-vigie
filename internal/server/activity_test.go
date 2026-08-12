package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

func TestActivityShellSurvivesIdle(t *testing.T) {
	var sess store.Session

	// The watcher reports a session that dropped to a shell: idle, but DETAIL = "shell".
	// The #236 rule blanks Activity on idle — shell is the deliberate exception (#280).
	sess = applyReport(sess, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Detail: "shell", Timestamp: "t1",
	})
	if sess.Detail != "shell" {
		t.Fatalf("activity = %q, want shell to survive idle (#280)", sess.Detail)
	}

	// The shell ends: idle with no shell message → it clears, no stale "shell".
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Timestamp: "t2",
	})
	if sess.Detail != "" {
		t.Errorf("activity = %q, want cleared once the shell ends (#280)", sess.Detail)
	}
}

func TestActivityClearedOnStatusChange(t *testing.T) {
	var sess store.Session

	// A working report carries a "doing" message.
	sess = applyReport(sess, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "working", Detail: "Edit render.go", Timestamp: "t1",
	})
	if sess.Detail != "Edit render.go" {
		t.Fatalf("activity = %q, want Edit render.go", sess.Detail)
	}

	// Same status, no new message → keep it (don't blank mid-turn).
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "working", Timestamp: "t2",
	})
	if sess.Detail != "Edit render.go" {
		t.Errorf("activity = %q, want it kept while status is unchanged", sess.Detail)
	}

	// Status changes and the report carries none → clear it (no stale message).
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Timestamp: "t3",
	})
	if sess.Detail != "" {
		t.Errorf("activity = %q, want cleared on a status change", sess.Detail)
	}
}

// TestLegacyActivityFieldStillAccepted covers the #393 compatibility path: the
// hook reporter is deliberately ungated by the version check, so it can lag the
// daemon. A report that still carries the pre-rename field must not lose its
// detail silently.
func TestLegacyActivityFieldStillAccepted(t *testing.T) {
	sess := applyReport(store.Session{}, true, api.ReportRequest{
		Event: "PostToolUse", SessionID: "s1", Activity: "Edit render.go",
		Timestamp: "2026-08-12T10:00:00Z",
	})
	if sess.Detail != "Edit render.go" {
		t.Errorf("detail = %q, want the legacy activity field to be read", sess.Detail)
	}
	// The new field wins when both are present.
	sess = applyReport(store.Session{}, true, api.ReportRequest{
		Event: "PostToolUse", SessionID: "s1", Detail: "new", Activity: "old",
		Timestamp: "2026-08-12T10:00:00Z",
	})
	if sess.Detail != "new" {
		t.Errorf("detail = %q, want the new field to win", sess.Detail)
	}
}
