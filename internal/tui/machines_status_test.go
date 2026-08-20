package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
)

// #509. aggregateMachines counted four statuses in a hand-written switch;
// `thinking`, `compacting`, `stalled`, `error` and `stale` fell through. They
// still incremented SESS, so a machine running three stalled sessions rendered
//
//	3 sessions   working 0   waiting 0   idle 0   ended 0
//
// Five of nine invisible, including two of the three the attention set is made
// of. The failure is silent by construction: the totals still add up on SESS, so
// nothing looks wrong.
//
// This is #421/#423 in the one client that never got wired to
// `internal/status` — the package that exists so a vocabulary is not copied by
// hand a fourth time.

// oneOfEachStatus is a machine running exactly one session per known status.
func oneOfEachStatus() []api.SessionView {
	out := make([]api.SessionView, 0, len(status.All))
	for i, st := range status.All {
		out = append(out, api.SessionView{ID: string(rune('a' + i)), Machine: "orion", Status: st})
	}
	return out
}

// The load-bearing assertion: a session counted in SESS is visible somewhere —
// either in a column of its own, or spelled out at the end of the row.
func TestNoStatusIsCountedButInvisible(t *testing.T) {
	sessions := oneOfEachStatus()
	out := renderMachines(sessions, map[string]string{"orion": "2026-08-16T12:00:00Z"},
		map[string]api.VersionInfo{}, 300)

	for _, st := range status.All {
		if columnStatuses[st] {
			continue // carried by WORK / WAIT / IDLE / ENDED, asserted below
		}
		if !strings.Contains(out, st+" 1") {
			t.Errorf("a %q session is counted in SESS and appears nowhere on the tab:\n%s", st, out)
		}
	}
	// And the four with a column really carry their session, rather than the
	// column being present while the count reads zero — the original defect.
	for _, want := range []string{"     1"} {
		if strings.Count(out, want) < 4 {
			t.Errorf("the four status columns do not all show their session:\n%s", out)
		}
	}
}

// And the count is right, not merely present.
func TestTheMachineTotalMatchesWhatIsShown(t *testing.T) {
	agg := aggregateMachines(oneOfEachStatus())
	if len(agg) != 1 {
		t.Fatalf("%d machines, want 1", len(agg))
	}
	m := agg[0]
	if m.sessions != len(status.All) {
		t.Fatalf("SESS = %d, want %d", m.sessions, len(status.All))
	}
	shown := 0
	for _, st := range status.All {
		shown += m.byStatus[st]
	}
	if shown != m.sessions {
		t.Errorf("%d sessions counted, %d attributed to a status — %d vanish", m.sessions, shown, m.sessions-shown)
	}
}

// A status this build has never heard of must still be counted, not dropped: the
// server owns the vocabulary and a client may be older than it.
func TestAnUnknownStatusIsStillAccountedFor(t *testing.T) {
	agg := aggregateMachines([]api.SessionView{
		{ID: "a", Machine: "orion", Status: "quantum"},
		{ID: "b", Machine: "orion", Status: "idle"},
	})
	m := agg[0]
	if m.sessions != 2 {
		t.Fatalf("SESS = %d, want 2", m.sessions)
	}
	if m.byStatus["quantum"] != 1 {
		t.Errorf("an unknown status was dropped instead of counted: %v", m.byStatus)
	}
}

// The common case must not pay for the fix: a fleet whose statuses all have a
// column renders exactly as before, with no trailing overflow.
func TestAFleetWithinTheColumnsAddsNothing(t *testing.T) {
	out := renderMachines([]api.SessionView{
		{ID: "a", Machine: "orion", Status: "working"},
		{ID: "b", Machine: "orion", Status: "idle"},
	}, map[string]string{"orion": "2026-08-16T12:00:00Z"}, map[string]api.VersionInfo{}, 200)

	// The machine's own row, not the no-watcher banner below it — whose prose
	// legitimately contains the word "stale".
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "orion") && !strings.Contains(line, "no watcher") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("no machine row found:\n%s", out)
	}
	for _, absent := range []string{"stalled", "thinking", "compacting", "error", "stale"} {
		if strings.Contains(row, absent) {
			t.Errorf("the row mentions %q though no session has it:\n%s", absent, row)
		}
	}
}
