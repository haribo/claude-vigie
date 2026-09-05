package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// #739. A session ends two ways: Claude Code says so, or it goes with its
// process — the machine was shut down, the terminal closed, Claude killed. Only
// the first stamped an end time, so the case where nobody was there to close the
// session, which is the one an operator most needs a time for, was the one that
// carried none.
//
// `Last seen` does not answer in its place: it advances on every report, and the
// watcher keeps reporting a dead session for as long as its transcript is inside
// the scan window. The end time is the only field that stays put.

func watchStatus(sess store.Session, isNew bool, status, ts string) store.Session {
	return applyReport(sess, isNew, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: status, Timestamp: ts,
	})
}

func TestTheWatcherStampsTheEndItObserved(t *testing.T) {
	sess := watchStatus(store.Session{}, true, "working", "t1")
	sess = watchStatus(sess, false, "ended", "t2")

	if sess.Status != "ended" {
		t.Fatalf("setup: status %q, want ended", sess.Status)
	}
	if sess.EndedAt != "t2" {
		t.Errorf("ended_at = %q, want t2 — the row reads `ended` with no time to show for it", sess.EndedAt)
	}
}

// The end happened once. The watcher goes on reporting it every couple of
// seconds, and each of those reports moves `Last seen` — the end time must not
// follow, or it says "just now" for as long as the transcript is scanned.
func TestTheEndTimeStaysAtTheFirstObservation(t *testing.T) {
	sess := watchStatus(store.Session{}, true, "working", "t1")
	sess = watchStatus(sess, false, "ended", "t2")
	sess = watchStatus(sess, false, "ended", "t3")

	if sess.EndedAt != "t2" {
		t.Errorf("ended_at = %q, want t2 — a repeat observation moved when the session ended", sess.EndedAt)
	}
	if sess.LastSeenAt != "t3" {
		t.Errorf("last_seen_at = %q, want t3 — the heartbeat is what keeps moving", sess.LastSeenAt)
	}
}

// A clean end is unchanged, including the clear #722 added.
func TestACleanEndStillStampsAndStillClears(t *testing.T) {
	sess := hookReport(store.Session{}, true, "SessionStart", "t1")
	sess = hookReport(sess, false, "SessionEnd", "t2")
	if sess.EndedAt != "t2" {
		t.Fatalf("ended_at = %q, want t2", sess.EndedAt)
	}
	if got := hookReport(sess, false, "UserPromptSubmit", "t3"); got.EndedAt != "" {
		t.Errorf("ended_at = %q on a session that came back, want it cleared", got.EndedAt)
	}
}
