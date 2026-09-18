package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #851. Measured on real sessions (Claude Code v2.1.267), driven through a pty
// with the registry and the board sampled every 10 s:
//
//	what the session does                          registry   vigie
//	`!` — operator at the bash prompt, sleep 90     busy       idle
//	Monitor — shell run for Claude, sleep 90        shell      idle + detail `shell`
//
// The second row is the defect: ninety seconds of `idle` while a shell runs, and
// the transcript cannot help — a Monitor's launch is answered in about 1.8 s, so
// nothing is pending by the time the watcher looks.
//
// The first row is what settles the meaning of the word. #255 mapped `shell` to
// `idle` on the grounds that it marks an operator at a bash prompt; that operator
// reports `busy`. In this version `shell` says a shell is running *for* Claude.
func TestAShellRunningForClaudeIsWorking(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no compaction markers
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "shell"}}

	// Nothing pending: the Monitor that started this shell was answered long ago.
	status, detail, _, _ := resolveStatus(reg, nil, "s", &transcript.Info{}, 5*time.Minute, now.Add(-5*time.Minute), now)

	if status != "working" {
		t.Errorf("status = %q, want working — the board offers a session running a shell as free", status)
	}
	if detail != "shell" {
		t.Errorf("detail = %q, want shell — the row still says what kind of work it is (#280)", detail)
	}
}

// The closing edge, and the reason this needs no liveness cap: the registry leaves
// `shell` by itself the moment the command ends — measured at t=101 s for a 90 s
// command. Nothing is inferred to close it, so no `working` can latch (ADR-0015).
func TestTheRegistryLeavingShellEndsTheWork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "idle"}}

	status, detail, _, _ := resolveStatus(reg, nil, "s", &transcript.Info{}, 5*time.Minute, now.Add(-5*time.Minute), now)

	if status != "idle" {
		t.Errorf("status = %q, want idle once the registry has left shell", status)
	}
	if detail == "shell" {
		t.Error("detail still says shell after the command ended")
	}
}
