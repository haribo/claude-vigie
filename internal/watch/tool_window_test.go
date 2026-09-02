package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// What the watcher reports for a transcript frozen on an unanswered `tool_use`.
//
// #530 pinned this as a two-part rule: a five-minute grace period during which a
// slow tool read `working`, then `stalled` once the window elapsed. The header of
// this file used to explain why the window mattered — that without it "a build, a
// test suite or a long search" would be called hung within 45 s.
//
// That reasoning was right and its conclusion was half-measured. ADR-0012 removed
// the verdict entirely rather than delaying it: the pairing proves a call is
// outstanding, never that it is hung, and only the operator knows whether twelve
// minutes is normal for what they asked. So there is no threshold left to keep the
// window clear of — `TestTheWindowOutlastsTheStalledThreshold` guarded an
// invariant between two constants, one of which is gone, and went with it.
//
// `toolWindow` itself remains: it governs `activelyWorking`, which decides the
// base for a transcript with no parsed pending call.

// statusAt is what the watcher reports for a frozen transcript at a given age.
func statusAt(age time.Duration, info *transcript.Info) string {
	base := "idle"
	if activelyWorking(info.LastStopReason, age) {
		base = "working"
	}
	return refineWithTools(base, info, age)
}

// A command reads `working` for as long as it runs. This is ADR-0012's claim, at
// the seam where the old threshold lived.
func TestAnOutstandingToolIsWorkingAtEveryAge(t *testing.T) {
	frozen := &transcript.Info{PendingTool: "Bash", LastStopReason: "tool_use"}

	for _, age := range []time.Duration{
		5 * time.Second, 30 * time.Second, time.Minute, 3 * time.Minute,
		toolWindow + time.Minute, time.Hour, 6 * time.Hour,
	} {
		if got := statusAt(age, frozen); got != "working" {
			t.Errorf("at %s a command still outstanding reads %q, want working — an e2e suite is not a fault", age, got)
		}
	}
}

// A background task was never a hung tool at any age, and still is not.
func TestABackgroundTaskIsWorkingAtEveryAge(t *testing.T) {
	bg := &transcript.Info{BackgroundActive: true, LastStopReason: "tool_use"}
	for _, age := range []time.Duration{time.Second, toolWindow + time.Hour} {
		if got := statusAt(age, bg); got != "working" {
			t.Errorf("at %s a background task reads %q, want working", age, got)
		}
	}
}
