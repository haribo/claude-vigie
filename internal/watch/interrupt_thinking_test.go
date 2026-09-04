package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #721. A turn killed with Ctrl-C shows `interrupted`, so it is distinguishable
// from one that finished (#351, #659). Unless Claude was thinking when it was
// killed — which is the case people actually hit, because interrupting during a
// long silent reasoning pass is the reason to interrupt at all.
//
// The transcript then carries both signals: the session was interrupted, and the
// last thing Claude produced was a reasoning block. The thinking refinement runs
// first and turns the session `thinking`, and the interrupted marker only ever
// attaches to a resting session — so it is never shown.
//
// And it does not pass: nothing clears the thinking signal but a new assistant
// line, and a killed turn produces none. The board shows a session reasoning,
// indefinitely, after the operator stopped it.
func TestAnInterruptedTurnIsNotStillThinking(t *testing.T) {
	now := time.Now()
	killed := &transcript.Info{Interrupted: true, Thinking: true}
	// Claude Code is alive and reports the session at rest — what it says after a
	// turn is killed. Hours later, with nothing since: the state the session sits
	// in until the operator types again.
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "idle"}}

	status, detail, _ := resolveStatus(reg, nil, "s", killed, time.Hour, now.Add(-time.Hour), now)

	if status != "idle" {
		t.Errorf("status = %q for a turn the operator killed, want idle", status)
	}
	if detail != "interrupted" {
		t.Errorf("detail = %q, want interrupted — a killed turn reads like one still reasoning", detail)
	}
}

// The refinement itself is untouched: a live turn reasoning still reads
// `thinking`. Only a turn that was killed is excluded.
func TestALiveTurnStillReadsThinking(t *testing.T) {
	now := time.Now()
	reasoning := &transcript.Info{Thinking: true}
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "busy"}}

	status, _, _ := resolveStatus(reg, nil, "s", reasoning, 2*time.Second, now.Add(-2*time.Second), now)
	if status != "thinking" {
		t.Errorf("status = %q for a live reasoning turn, want thinking", status)
	}
}
