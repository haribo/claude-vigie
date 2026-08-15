package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #483, end to end: what the watcher actually reported for session 884e43f5 was
// "stalled — stopped at Monitor", at every pause, for the rest of the day. The
// transcript below is the observed shape of that failure; the assertion is the
// status the operator should have seen.
func TestASessionDoesNotStallOnADeadToolCall(t *testing.T) {
	lines := []string{
		// The tool call that was in flight when Claude Code died. No tool_result
		// ever follows it.
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Monitor","input":{}}]}}`,
		// An hour later: Claude Code's own resume line, then the operator's prompt.
		`{"type":"user","isMeta":true,"message":{"content":[{"type":"text","text":"Continue from where you left off."}]}}`,
		`{"type":"user","message":{"content":"ma session gnome a été coupée"}}`,
		// Dozens of complete turns, one of which is enough to make the point.
		`{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"t2","name":"Read","input":{}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2"}]}}`,
		`{"type":"assistant","message":{"id":"m3","content":[{"type":"text","text":"done"}]}}`,
	}
	path := filepath.Join(t.TempDir(), "orphan.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := transcript.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Quiet well past the threshold — the state the session sat in all day.
	if got := refineWithTools("idle", info, stalledAfter+time.Hour); got != "idle" {
		t.Errorf("status = %q, want idle (pending %q) — a dead tool call still calls the operator", got, info.PendingTool)
	}
}
