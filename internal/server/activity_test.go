package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

func TestActivityShellSurvivesIdle(t *testing.T) {
	var sess store.Session

	// The watcher reports a session that dropped to a shell: idle, but DOING = "shell".
	// The #236 rule blanks Activity on idle — shell is the deliberate exception (#280).
	sess = applyReport(sess, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Activity: "shell", Timestamp: "t1",
	})
	if sess.Activity != "shell" {
		t.Fatalf("activity = %q, want shell to survive idle (#280)", sess.Activity)
	}

	// The shell ends: idle with no shell message → it clears, no stale "shell".
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Timestamp: "t2",
	})
	if sess.Activity != "" {
		t.Errorf("activity = %q, want cleared once the shell ends (#280)", sess.Activity)
	}
}

func TestActivityClearedOnStatusChange(t *testing.T) {
	var sess store.Session

	// A working report carries a "doing" message.
	sess = applyReport(sess, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "working", Activity: "Edit render.go", Timestamp: "t1",
	})
	if sess.Activity != "Edit render.go" {
		t.Fatalf("activity = %q, want Edit render.go", sess.Activity)
	}

	// Same status, no new message → keep it (don't blank mid-turn).
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "working", Timestamp: "t2",
	})
	if sess.Activity != "Edit render.go" {
		t.Errorf("activity = %q, want it kept while status is unchanged", sess.Activity)
	}

	// Status changes and the report carries none → clear it (no stale message).
	sess = applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, Status: "idle", Timestamp: "t3",
	})
	if sess.Activity != "" {
		t.Errorf("activity = %q, want cleared on a status change", sess.Activity)
	}
}
