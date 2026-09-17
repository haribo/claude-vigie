package replay_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #834 across the seam. The defect is in what the watcher derives from a real
// transcript, and only the board shows it: a session watching a persistent
// `Monitor` read `idle`, so an operator scanning for a free session picked one that
// was occupied.
func (f *fleet) writePersistentMonitorTranscript(sessionID string, age time.Duration, now time.Time) {
	f.t.Helper()
	proj := filepath.Join(f.root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		f.t.Fatal(err)
	}
	stamp := now.Add(-age).UTC().Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"sessionId":%q,"cwd":"/work","type":"assistant","timestamp":%q,"message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_mon","name":"Monitor","input":{"command":"until curl -sf http://example.test/health; do sleep 5; done","description":"watch the health endpoint","persistent":true,"timeout_ms":600000}}],"usage":{"output_tokens":5}}}`, sessionID, stamp),
		// Answered within seconds, and the monitor runs on.
		fmt.Sprintf(`{"sessionId":%q,"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_mon","content":"Monitor started (task bq7x1a2c3, persistent — runs until TaskStop or session end). You will be notified on each match."}]}}`, sessionID, stamp),
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

func TestASessionWatchingAPersistentMonitorIsNotOffered(t *testing.T) {
	const id = "s-monitor"
	f := newFleet(t)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	// The registry says `idle`: Claude is producing nothing, which is true — the
	// work is the monitor, and only the transcript knows about it.
	f.writeRegistry(id, "idle", "")
	f.writePersistentMonitorTranscript(id, 30*time.Second, now)
	f.scanAndReport(now)

	if got := f.statusOf(id); got != "working" {
		t.Errorf("the board says %q, want working — the monitor is still running, so "+
			"the session is occupied and must not be offered as free (#834, #748)", got)
	}
}
