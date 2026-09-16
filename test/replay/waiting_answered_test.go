package replay_test

import (
	"testing"
	"time"
)

// #816 replayed across the seam, which is the only place it was ever visible.
//
// The operator answers a permission prompt and Claude runs the tool. Nothing
// reports that: no `PreToolUse` hook is installed, `PostToolUse` fires only when
// the tool is done, and the transcript is frozen on the `tool_use` line for the
// whole run. The one observer left is Claude Code's registry, which says `busy` —
// and `holdsWaiting` held it off exactly like a guess at silence, because it never
// read `StatusDeclared`.
//
// Every half of this passed its own tests while the defect was live. The watcher's
// suite proved it derives `working` from that registry; the server's suite proved
// its guards behave as written. Only the two together say what the operator sees.
func TestASessionAnsweredStopsReadingWaiting(t *testing.T) {
	const id = "s-answered"
	f := newFleet(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	// 1. Claude asks. The Notification hook posts `waiting`, and the hook owns it.
	f.writeTranscript(id, 30*time.Second, now)
	f.writeRegistry(id, "waiting", "permit tool")
	f.hook(id, "Notification", "permission_prompt", now)
	f.scanAndReport(now)
	if got := f.statusOf(id); got != "waiting" {
		t.Fatalf("while the prompt is up the board says %q, want waiting", got)
	}

	// 2. The operator answers. Claude Code's registry says so; the transcript does
	//    not move, because the tool is running and writes nothing until it ends.
	f.writeRegistry(id, "busy", "")
	f.scanAndReport(now.Add(2 * time.Second))

	if got := f.statusOf(id); got != "working" {
		t.Errorf("after the prompt was answered the board says %q, want working — "+
			"the session is working and needs nobody, yet it sits in the attention "+
			"set for as long as the command runs (#816)", got)
	}
}

// The guard that must survive it, replayed the same way: while the prompt is
// genuinely up, a *frozen transcript* must not release the waiting. To the watcher
// a running tool and a blocking prompt are the same silence — which is why the
// release has to come from what Claude Code states, never from what the watcher
// deduces (#235, #508).
func TestASessionStillAskingKeepsWaiting(t *testing.T) {
	const id = "s-asking"
	f := newFleet(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	f.writeTranscript(id, 30*time.Second, now)
	f.writeRegistry(id, "waiting", "permit tool")
	f.hook(id, "Notification", "permission_prompt", now)
	f.scanAndReport(now)

	// Time passes and nothing moves: the prompt is still on the operator's screen.
	for _, d := range []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second} {
		f.scanAndReport(now.Add(d))
		if got := f.statusOf(id); got != "waiting" {
			t.Fatalf("after %s the board says %q, want waiting — the prompt is still up "+
				"and nothing but Claude Code saying otherwise may clear it", d, got)
		}
	}
}
