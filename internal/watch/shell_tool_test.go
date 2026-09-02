package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #661. Claude Code's registry says `shell` for two situations it names alike:
// the operator dropped to a shell prompt inside Claude — alive, producing
// nothing, a real rest (#280) — and a Bash tool executing, which is work in
// progress. Measured on a live session: the registry sat at `shell` for 78 s of a
// two-minute window while a foreground command ran.
//
// vigie read both as `idle`, so a session was reported doing nothing while its
// build ran; and because `idle` is the base the tool pairing acts on, the same
// build was reported `stalled` after 45 s — the false positive
// session-status.md § 2 says the five-minute window exists to prevent.
//
// The transcript tells the two apart: an unanswered `tool_use` means Claude is
// waiting on a command.
//
// These enter through resolveStatus on purpose. tool_window_test.go rebuilds the
// base by hand and so passes on a path no current client is on.
func TestAShellRunningAToolIsWorking(t *testing.T) {
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}
	running := &transcript.Info{PendingTool: "Bash", LastStopReason: "tool_use", Activity: "Bash: run the e2e suite"}

	for _, age := range []time.Duration{5 * time.Second, 46 * time.Second, 3 * time.Minute, time.Hour} {
		status, detail, _ := resolveStatus(reg, nil, "s", running, age, now.Add(-age), now)
		if status != "working" {
			t.Errorf("a command running for %s reads %q, want working — the session is waiting on it", age, status)
		}
		if detail == "shell" {
			t.Errorf("at %s the DETAIL says %q; the tool is more use than the word shell", age, detail)
		}
	}
}

// The original meaning is untouched: an operator at a shell prompt, with no tool
// outstanding, is resting. Inventing `working` for someone typing at bash would
// be the same error in the other direction.
func TestAShellPromptWithNoToolIsStillIdle(t *testing.T) {
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}
	resting := &transcript.Info{}

	status, detail, _ := resolveStatus(reg, nil, "s", resting, 10*time.Minute, now.Add(-10*time.Minute), now)
	if status != "idle" || detail != "shell" {
		t.Errorf("a shell prompt reads (%q, %q), want (idle, shell) — #280 is not what this changes", status, detail)
	}
}
