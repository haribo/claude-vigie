package replay_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #818 replayed across the same seam, and for the same reason: the defect was in
// what the watcher derives from a real transcript, and only the board shows it.
//
// A `<task-notification>` has two deliveries. When the command ends while the
// session is at rest it arrives as a `user` line; when it ends **while a turn is
// still running** — the normal case for something launched to outlive the turn —
// Claude Code queues it, and the same text arrives on an `attachment` line. The
// parser had no case for that line, so the command stayed open for the rest of the
// session and the board read `working` with nothing able to clear it.
func (f *fleet) writeBackgroundTranscript(sessionID string, closedByAttachment bool, age time.Duration, now time.Time) {
	f.t.Helper()
	proj := filepath.Join(f.root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		f.t.Fatal(err)
	}
	stamp := now.Add(-age).UTC().Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"sessionId":%q,"cwd":"/work","type":"assistant","timestamp":%q,"message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"just code-check","run_in_background":true}}],"usage":{"output_tokens":5}}}`, sessionID, stamp),
		// Claude Code answers the launch within seconds, while the command runs.
		fmt.Sprintf(`{"sessionId":%q,"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg","content":"Command running in background with ID: bq7x1a2c3"}]}}`, sessionID, stamp),
	}
	if closedByAttachment {
		lines = append(lines, fmt.Sprintf(
			`{"sessionId":%q,"type":"attachment","timestamp":%q,"attachment":{"type":"queued_command","commandMode":"task-notification","prompt":"<task-notification>\n<tool-use-id>toolu_bg</tool-use-id>\n<status>completed</status>\n<summary>done</summary>\n</task-notification>"}}`,
			sessionID, stamp))
	}
	p := filepath.Join(proj, sessionID+".jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
		f.t.Fatal(err)
	}
}

func TestABackgroundCommandClosedByAnAttachmentLeavesTheBoard(t *testing.T) {
	const id = "s-background"
	f := newFleet(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	f.writeRegistry(id, "idle", "")

	// While it runs, the board must say so: that part was never broken, and pinning
	// it here keeps the next assertion from passing for the wrong reason.
	f.writeBackgroundTranscript(id, false, 30*time.Second, now)
	f.scanAndReport(now)
	if got := f.statusOf(id); got != "working" {
		t.Fatalf("while the command runs the board says %q, want working", got)
	}

	// The command ends mid-turn, so its notification is queued as an attachment.
	f.writeBackgroundTranscript(id, true, 5*time.Second, now)
	f.scanAndReport(now.Add(time.Second))
	if got := f.statusOf(id); got != "idle" {
		t.Errorf("after the command finished the board says %q, want idle — its own "+
			"notification was delivered and ignored, and no prompt can clear it (#812, #818)", got)
	}
}
