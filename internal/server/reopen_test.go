package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

// #664. A session that ended and is then resumed — `claude --resume` keeps the
// same id — came back reading `ended`, still carrying the timestamp of the end it
// had already left behind.
//
// `SessionStart` preserving the current status is right for every other value it
// can meet: the event says a session exists, not what it is doing. `ended` is the
// one value the event is direct evidence against, and `EndedAt` was written once
// and never cleared, so the board kept a fact that had stopped being true.

func hookReport(sess store.Session, isNew bool, event, ts string) store.Session {
	return applyReport(sess, isNew, api.ReportRequest{
		SessionID: "s", Event: event, Timestamp: ts,
	})
}

func TestSessionStartReopensAnEndedSession(t *testing.T) {
	sess := hookReport(store.Session{}, true, "SessionStart", "t1")
	sess = hookReport(sess, false, "SessionEnd", "t2")
	if sess.Status != "ended" || sess.EndedAt != "t2" {
		t.Fatalf("after SessionEnd: status %q, ended_at %q — want ended at t2", sess.Status, sess.EndedAt)
	}

	// The operator resumes it. This is the event that says so.
	sess = hookReport(sess, false, "SessionStart", "t3")
	if sess.Status != "idle" {
		t.Errorf("status = %q after a resume, want idle — the board says the session is over while it is open", sess.Status)
	}
	if sess.EndedAt != "" {
		t.Errorf("ended_at = %q after a resume, want it cleared — it is what every client is handed", sess.EndedAt)
	}
}

// The rule is narrow on purpose: `SessionStart` still says nothing about a
// session that is doing something.
func TestSessionStartLeavesALiveStatusAlone(t *testing.T) {
	for _, status := range []string{"working", "waiting", "idle", "error", "stalled"} {
		sess := store.Session{Status: status, StatusSource: "hook"}
		got := hookReport(sess, false, "SessionStart", "t1")
		if got.Status != status {
			t.Errorf("SessionStart turned %q into %q", status, got.Status)
		}
	}
}

// A watcher that has decided the process is gone must still be overridable by the
// hook that proves otherwise — the hook runs inside the session, the watcher
// infers from outside.
func TestSessionStartReopensWhatTheWatcherEnded(t *testing.T) {
	sess := applyReport(store.Session{}, true, api.ReportRequest{
		SessionID: "s", Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Status: "ended", Timestamp: "t1",
	})
	if sess.Status != "ended" {
		t.Fatalf("setup: status %q, want ended", sess.Status)
	}
	if got := hookReport(sess, false, "SessionStart", "t2"); got.Status != "idle" {
		t.Errorf("status = %q, want idle — a hook inside the session outranks an inference from outside", got.Status)
	}
}
