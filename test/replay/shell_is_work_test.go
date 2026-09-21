package replay_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFinishedTurnTranscript writes a transcript whose last turn is over: no
// tool_use left unanswered, nothing for the transcript-side rules to hold. It is
// the shape a `Monitor` leaves behind — its launch is answered in about 1.8 s — and
// the shape that makes the registry word the only remaining evidence.
func (f *fleet) writeFinishedTurnTranscript(sessionID string, age time.Duration, now time.Time) {
	f.t.Helper()
	proj := filepath.Join(f.root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		f.t.Fatal(err)
	}
	stamp := now.Add(-age).UTC().Format(time.RFC3339)
	line := fmt.Sprintf(
		`{"sessionId":%q,"cwd":"/work","type":"assistant","timestamp":%q,`+
			`"message":{"id":"m1","stop_reason":"end_turn","content":[{"type":"text","text":"Moniteur réarmé."}],"usage":{"output_tokens":5}}}`,
		sessionID, stamp)
	p := filepath.Join(proj, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(line+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
		f.t.Fatal(err)
	}
}

// #851 across the seam, replaying what was measured on a real session.
//
// Claude Code v2.1.267, driven through a pty with the registry and the board
// sampled every 10 s: a `Monitor` running `sleep 90` put the registry at `shell`
// for the whole ninety seconds while the board said `idle`, and at t=101 s the
// registry returned to `idle` on its own, the moment the command ended.
//
// Both halves matter and only the second makes the first safe: `shell` is work,
// and the registry itself says when that work is over — so reading it as `working`
// cannot latch a session busy forever (ADR-0017, ADR-0015).
//
// The transcript is deliberately a plain finished turn. That is the shape the
// exceptions built for #661 and #834 cannot see: a Monitor's launch is answered in
// about 1.8 s, so nothing is left pending by the time the watcher looks, and only
// the registry word is left to carry the fact that a shell is running.
func TestAShellRunningForClaudeIsNotOfferedAsFree(t *testing.T) {
	const id = "s-shell"
	f := newFleet(t)
	now := time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)

	// t+10s of the measured run: the shell is running, the transcript is quiet.
	f.writeRegistry(id, "shell", "")
	f.writeFinishedTurnTranscript(id, 30*time.Second, now)
	f.scanAndReport(now)

	if got := f.statusOf(id); got != "working" {
		t.Errorf("the board says %q while a shell runs for Claude, want working — "+
			"an operator scanning for a free session picks one that is busy (#851)", got)
	}

	// t+101s: the command ended and the registry left `shell` by itself.
	later := now.Add(90 * time.Second)
	f.writeRegistry(id, "idle", "")
	f.scanAndReport(later)

	if got := f.statusOf(id); got != "idle" {
		t.Errorf("the board says %q after the registry left shell, want idle — "+
			"a status that cannot be left is the latch ADR-0015 refuses", got)
	}
}
