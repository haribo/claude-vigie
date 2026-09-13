package server

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/store"
)

// #821. Two guards defend the same property — a hook-set status against a watcher
// report. `reconcileWatch` resolves authority; `holdsWaiting` holds a `waiting`
// while the transcript is frozen. `applyStatus` calls the second one first.
//
// #816 was created by teaching one of them a new distinction and not the other:
// `f2fad0c` (#811) added `StatusDeclared` to `reconcileWatch`, and `holdsWaiting`
// went on treating a status Claude Code had *stated* exactly like one the watcher
// had *guessed*. It went unnoticed because the two guards do not cover the same
// set — `waiting` is the only status covered by both, so it is the only one where
// the untaught guard still decided.
//
// The suite could not see it. `observed_test.go` is complete about the declared
// distinction and `waiting_inferred_test.go` is complete about the frozen-transcript
// hold; **the tests were organized per guard, so no test asked what the system
// answers for a given combination.** The cell nobody owned was `waiting` × declared.
//
// This is that missing test, and it is not a golden snapshot: freezing what the
// code does today would have frozen #816 as correct. `specDecide` below is
// `docs/design/session-status.md` § 3 written as a function, from the document —
// so a disagreement means the code contradicts the specification, whichever of
// the two is wrong.

// specDecide is § 3 read as a function. Each branch names the paragraph it encodes.
//
// **If you change this to match new code, you are asserting the specification
// changed. Go and change the document, in the same commit.** The point of the test
// is that the two cannot drift apart quietly, and editing this function to agree
// with the implementation is precisely how that protection is lost.
func specDecide(current, source, incoming string, declared, tsAfter, hasChangedAt bool) (string, string) {
	// "A `waiting` the watcher only *inferred* is cleared once the transcript
	// moves." Any status inferred from silence is held; `error` and `ended` are
	// positive observations and still win. With no timestamp there is no evidence
	// the transcript moved, so the hold stands (#636).
	if current == "waiting" && source == "hook" && hasChangedAt && !declared &&
		incoming != "error" && incoming != "ended" && !tsAfter {
		return "waiting", "hook"
	}
	// "An `idle` the watcher *inferred* from a quiet transcript must not retract a
	// hook-owned `waiting`, `working`, or `thinking`."
	if incoming == "idle" && !declared && source == "hook" &&
		(current == "waiting" || current == "working" || current == "thinking") {
		return current, "hook"
	}
	// "A report that merely confirms the current status keeps the current owner."
	if incoming == current {
		return current, source
	}
	// "Any such change wins and becomes watch-owned" — and a declared status
	// reaches here rather than being held, which is "an observation beats a
	// deduction".
	return incoming, "watch"
}

func TestTheCodeAndTheSpecificationAgreeOnEveryCell(t *testing.T) {
	// Both dimensions come from the vocabulary itself, never a list written here:
	// a status added to `status.All` grows this table on its own. A hand-kept list
	// is the failure mode the whole issue is about, and it would be absurd to
	// reintroduce it in the test that guards against it.
	const changedAt = "2026-09-13T12:00:00Z"
	stamps := []struct {
		name         string
		ts           string
		after        bool
		hasChangedAt bool
	}{
		{"newer than the status", "2026-09-13T12:00:01Z", true, true},
		{"older than the status", "2026-09-13T11:59:59Z", false, true},
		{"no status change recorded", "", false, false},
	}

	cells := 0
	for _, current := range status.All {
		for _, source := range []string{"hook", "watch"} {
			for _, incoming := range status.All {
				for _, declared := range []bool{true, false} {
					for _, st := range stamps {
						cells++
						recorded := changedAt
						if !st.hasChangedAt {
							recorded = ""
						}
						got := applyStatus(
							store.Session{ID: "s", Status: current, StatusSource: source, StatusChangedAt: recorded},
							api.ReportRequest{
								SessionID: "s", Event: "watch", Status: incoming,
								StatusDeclared: declared, Timestamp: st.ts,
							})
						wantStatus, wantSource := specDecide(current, source, incoming, declared, st.after, st.hasChangedAt)
						if got.Status != wantStatus || got.StatusSource != wantSource {
							t.Errorf("current=%s/%s incoming=%s declared=%v timestamp=%s\n"+
								"  code says %s/%s, the specification says %s/%s\n"+
								"  one of the two is wrong; fix the code, or change the document and this function together",
								current, source, incoming, declared, st.name,
								got.Status, got.StatusSource, wantStatus, wantSource)
						}
					}
				}
			}
		}
	}
	// Stated so that a surface which silently shrinks is visible: a vocabulary
	// pruned without anyone noticing would make this test pass by covering less.
	if want := len(status.All) * 2 * len(status.All) * 2 * len(stamps); cells != want {
		t.Errorf("covered %d cells, want %d", cells, want)
	}
	t.Logf("%d cells of the reconciliation surface compared against § 3", cells)
}
