package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// #530. session-status.md § 2 said the tool_use↔tool_result pairing "replaces the
// old blind 5-minute window". It does not — it completes it, and the design has
// been amended to say so.
//
// The window is the grace period: a tool call may legitimately run that long.
// Removing it would drop the session to an idle base after 10 s, and the pairing
// would then report `stalled` at 45 s for every build, test suite or long search
// — a false positive on one of the statuses that call the operator.
//
// What the pairing adds is what happens *after* the window: a turn stopped on a
// tool used to fall to `idle`, so a hung tool looked exactly like a finished
// turn. This pins both halves, so the code and the spec cannot drift apart again
// in silence.

// statusAt is what the watcher reports for a transcript frozen on an unanswered
// tool_use, at a given age.
func statusAt(age time.Duration, info *transcript.Info) string {
	base := "idle"
	if activelyWorking(info.LastStopReason, age) {
		base = "working"
	}
	return refineWithTools(base, info, age)
}

func TestASlowToolIsWorkingAndAHungOneIsStalled(t *testing.T) {
	frozen := &transcript.Info{PendingTool: "Bash", LastStopReason: "tool_use"}

	for _, age := range []time.Duration{5 * time.Second, 30 * time.Second, time.Minute, 3 * time.Minute} {
		if got := statusAt(age, frozen); got != "working" {
			t.Errorf("at %s a tool still within its window reads %q, want working — a build would be reported as hung", age, got)
		}
	}
	if got := statusAt(toolWindow+time.Minute, frozen); got != "stalled" {
		t.Errorf("past the window a tool that never answered reads %q, want stalled — it would look like a finished turn", got)
	}
}

// The grace period is what separates the two, so it must stay clear of the
// threshold the pairing uses: with toolWindow below stalledAfter there would be
// no window at all.
func TestTheWindowOutlastsTheStalledThreshold(t *testing.T) {
	if toolWindow <= stalledAfter {
		t.Errorf("toolWindow %s is not longer than stalledAfter %s — every slow tool would be reported as hung", toolWindow, stalledAfter)
	}
}

// A background task is not a hung tool at any age: it legitimately runs long, and
// that is decided by the pairing rather than by the window.
func TestABackgroundTaskIsNeverStalled(t *testing.T) {
	bg := &transcript.Info{BackgroundActive: true, LastStopReason: "tool_use"}
	for _, age := range []time.Duration{time.Second, toolWindow + time.Hour} {
		if got := statusAt(age, bg); got != "working" {
			t.Errorf("at %s a background task reads %q, want working", age, got)
		}
	}
}
