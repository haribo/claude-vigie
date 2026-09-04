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

// #722. The end time is stamped when a session ends and was cleared only by a
// `SessionStart`. But that is not the only way a session comes back: typing a
// prompt does it too, and so does a watcher report finding the process alive.
// The value is served to every client, so a session demonstrably running carried
// a timestamp saying when it stopped.
//
// #664 fixed the status and the one event that clearly contradicts it. It did not
// ask what else contradicts it.
func TestTheEndTimeGoesWheneverTheSessionComesBack(t *testing.T) {
	ended := func(t *testing.T) store.Session {
		t.Helper()
		s := hookReport(store.Session{}, true, "SessionStart", "t1")
		s = hookReport(s, false, "SessionEnd", "t2")
		if s.Status != "ended" || s.EndedAt != "t2" {
			t.Fatalf("setup: status %q, ended_at %q", s.Status, s.EndedAt)
		}
		return s
	}

	// A prompt: the session is working again, and it did not stop at t2 after all.
	if got := hookReport(ended(t), false, "UserPromptSubmit", "t3"); got.Status != "working" || got.EndedAt != "" {
		t.Errorf("after a prompt: status %q, ended_at %q — want working and no end time", got.Status, got.EndedAt)
	}

	// And an end time is kept for as long as the session is over: a report that
	// leaves it ended must not erase when it ended.
	if got := hookReport(ended(t), false, "SessionEnd", "t4"); got.EndedAt == "" {
		t.Error("the end time was cleared on a session that is still over")
	}
}
