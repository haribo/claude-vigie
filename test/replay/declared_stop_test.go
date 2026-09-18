package replay_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #842 across the seam. The session stopped its own background command and the
// board went on offering it as busy for the rest of its life — visible only here,
// where a real transcript drives the real watcher into the real server.
func (f *fleet) writeStoppedLaunchTranscript(sessionID string, stopped bool, age time.Duration, now time.Time) {
	f.t.Helper()
	proj := filepath.Join(f.root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		f.t.Fatal(err)
	}
	stamp := now.Add(-age).UTC().Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"sessionId":%q,"cwd":"/work","type":"assistant","timestamp":%q,"message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"just code-check","run_in_background":true}}],"usage":{"output_tokens":5}}}`, sessionID, stamp),
		fmt.Sprintf(`{"sessionId":%q,"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg","content":"Command running in background with ID: bq7x1a2c3. Output is being written to /tmp/example/bq7x1a2c3.output"}]}}`, sessionID, stamp),
	}
	if stopped {
		lines = append(lines,
			fmt.Sprintf(`{"sessionId":%q,"type":"assistant","timestamp":%q,"message":{"id":"m2","content":[{"type":"tool_use","id":"toolu_stop","name":"TaskStop","input":{"task_id":"bq7x1a2c3"}}]}}`, sessionID, stamp),
			fmt.Sprintf(`{"sessionId":%q,"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stop","content":"{\"message\":\"Successfully stopped task: bq7x1a2c3 (just code-check)\"}"}]}}`, sessionID, stamp))
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	p := filepath.Join(proj, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
		f.t.Fatal(err)
	}
}

func TestASessionThatStoppedItsOwnCommandLeavesTheBoard(t *testing.T) {
	const id = "s-stopped"
	f := newFleet(t)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	f.writeRegistry(id, "idle", "")

	// While it runs the board must say so — pinned first, or the assertion below
	// could pass for the wrong reason.
	f.writeStoppedLaunchTranscript(id, false, 30*time.Second, now)
	f.scanAndReport(now)
	if got := f.statusOf(id); got != "working" {
		t.Fatalf("while the command runs the board says %q, want working", got)
	}

	// The session stops it. Claude Code answers the stop and emits no notification
	// for a task it was told to stop, so nothing else will ever close this launch.
	f.writeStoppedLaunchTranscript(id, true, 5*time.Second, now)
	f.scanAndReport(now.Add(time.Second))
	if got := f.statusOf(id); got != "idle" {
		t.Errorf("after the session stopped its own command the board says %q, want idle — "+
			"the stop is in the transcript and no prompt can clear the status (#810, #812, #842)", got)
	}
}
