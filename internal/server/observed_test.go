package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// #803. A session finished its turn while the daemon was unreachable, so the
// `Stop` hook never posted — hooks do not queue, and one must not pay the
// deadline on every tool call for a daemon already found unreachable (#578). The
// link came back, the watcher reported `idle`, and the daemon refused it: a hook
// outranks the watcher.
//
// The rule is right and its premise had gone. A hook outranks the watcher because
// it speaks from inside the session — but this one observed nothing, it was
// silenced, and the status it left behind is an hour old.
//
// So the rule is stated the way it was always meant: **what was observed beats
// what was inferred**. A watcher `idle` read from Claude Code's own session
// record is a statement by Claude Code, and it outranks a stale hook. A watcher
// `idle` inferred from a quiet transcript does not — that is where a permission
// prompt and a running tool look the same (#235), which is what the guard exists
// for.

func TestADeclaredIdleClearsAStaleHookWorking(t *testing.T) {
	// A prompt: the hook owns `working`.
	sess := hookReport(store.Session{}, true, "UserPromptSubmit", "t1")
	if sess.Status != "working" || sess.StatusSource != "hook" {
		t.Fatalf("setup: %q from %q", sess.Status, sess.StatusSource)
	}

	// The outage swallows the Stop. The watcher then reports what Claude Code's
	// own record says: this session is at rest.
	got := applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "idle", StatusDeclared: true, Timestamp: "t2",
	})
	if got.Status != "idle" {
		t.Errorf("status = %q, want idle — the board shows a session busy that has been waiting since the outage", got.Status)
	}
}

// The guard it must not break: an `idle` the watcher only inferred from silence
// still loses to a hook, because a permission prompt looks exactly like that.
func TestAnInferredIdleStillLosesToAHook(t *testing.T) {
	for _, held := range []string{"working", "waiting", "thinking"} {
		sess := store.Session{ID: "s", Status: held, StatusSource: "hook"}
		got := applyReport(sess, false, api.ReportRequest{
			SessionID: "s", Event: "watch", Status: "idle", Timestamp: "t2",
		})
		if got.Status != held {
			t.Errorf("a guess at silence overrode a hook-set %q → %q", held, got.Status)
		}
	}
}

// And a declared status the hook never contradicted is unaffected either way.
func TestADeclaredStatusIsOtherwiseOrdinary(t *testing.T) {
	sess := store.Session{ID: "s", Status: "idle", StatusSource: "watch"}
	got := applyReport(sess, false, api.ReportRequest{
		SessionID: "s", Event: "watch", Status: "working", StatusDeclared: true, Timestamp: "t2",
	})
	if got.Status != "working" || got.StatusSource != "watch" {
		t.Errorf("(%q, %q), want (working, watch)", got.Status, got.StatusSource)
	}
}

// #816. The same rule, on the status it matters most for.
//
// A permission prompt sets `waiting` from the Notification hook. The operator
// answers, Claude runs the tool — and nothing reports that: no `PreToolUse` hook
// is installed, `PostToolUse` fires only when the tool is done, and the transcript
// freezes on the `tool_use` line for the whole run (measured: no dated line at all
// in 770 of 1193 long tool calls in the local corpus). So the only observer left
// is the registry, which says `busy` — and `holdsWaiting` held it off exactly like
// a guess at silence, because it never read `StatusDeclared`.
//
// The board then read `waiting` for as long as the command ran: an attention
// state, on a session that is working and needs nobody. The GNOME badge counts it,
// because that count is recomputed from the current statuses rather than fired on
// a transition.
//
// The guard this must not break is the reason `holdsWaiting` exists (#235, #508):
// to the watcher, a running tool and a blocking prompt are the same frozen
// transcript. That is true of what it *deduces* from silence, and the test below
// pins it. It is not true of what Claude Code *states*: its registry has a
// `waiting` value of its own, carrying the question in `waitingFor`, so a declared
// `busy` is a positive statement that the operator is not the blocker.
func TestADeclaredStatusClearsAStaleHookWaiting(t *testing.T) {
	for _, c := range []struct {
		declared string
		want     string
		why      string
	}{
		{"working", "working", "the operator answered and the tool is running"},
		{"idle", "idle", "the outage swallowed the Stop and the session is at rest"},
	} {
		t.Run(c.declared, func(t *testing.T) {
			// The prompt: the hook owns `waiting`, posted at t2.
			sess := store.Session{
				ID: "s", Status: "waiting", StatusSource: "hook",
				StatusChangedAt: "2026-09-10T12:00:02Z",
			}
			// The registry states otherwise. Its report carries the transcript
			// mtime, which is the `tool_use` line — older than the prompt, and it
			// stays older for as long as the tool runs.
			got := applyReport(sess, false, api.ReportRequest{
				SessionID: "s", Event: "watch", Status: c.declared, StatusDeclared: true,
				Timestamp: "2026-09-10T12:00:01Z",
			})
			if got.Status != c.want {
				t.Errorf("status = %q, want %q — %s", got.Status, c.want, c.why)
			}
		})
	}
}

// And the guard that must stay: a status the watcher only *inferred* from a
// frozen transcript still loses to a hook `waiting`, because that is exactly what
// a permission prompt looks like from the outside (#235, #508).
func TestAnInferredStatusStillHoldsAHookWaiting(t *testing.T) {
	for _, inferred := range []string{"working", "thinking", "compacting", "idle"} {
		sess := store.Session{
			ID: "s", Status: "waiting", StatusSource: "hook",
			StatusChangedAt: "2026-09-10T12:00:02Z",
		}
		got := applyReport(sess, false, api.ReportRequest{
			SessionID: "s", Event: "watch", Status: inferred,
			Timestamp: "2026-09-10T12:00:01Z",
		})
		if got.Status != "waiting" {
			t.Errorf("a guess at silence (%q) cleared a hook waiting → %q", inferred, got.Status)
		}
	}
}
