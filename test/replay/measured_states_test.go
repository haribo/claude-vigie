package replay_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// The shapes `tools/drive` confirmed against a real Claude Code (v2.1.267), frozen
// here so they cannot regress for free (#855). Each table below is what a run
// printed; the assertion is that the same registry word still produces the same
// row.
//
// This is the loop the harness exists for: measure once, where only a real session
// can answer, then let CI keep it.

// Measured, scenario `waiting`:
//
//	t     registry   vigie     detail
//	10s   waiting    waiting   Bash: Create an empty probe file
//
// It matters more than it looks. The server's `holdsWaiting` rests on the registry
// carrying a `waiting` of its own, and that had been seen exactly once — by
// `tools/capture` during #817. Its own comment says that had the registry reported
// `busy` through a prompt instead, the rule would be a regression rather than a
// fix. This is the second sighting, and the first that CI keeps.
func TestARegistryWaitingReachesTheBoardAsWaiting(t *testing.T) {
	const id = "s-waiting"
	f := newFleet(t)
	now := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)

	f.writeRegistry(id, "waiting", "Bash: Create an empty probe file")
	f.writeTranscript(id, 20*time.Second, now)
	f.scanAndReport(now)

	if got := f.statusOf(id); got != "waiting" {
		t.Errorf("the board says %q while Claude is asking the operator, want waiting — "+
			"waiting is one of the two statuses that call a human (#855)", got)
	}
}

// Measured, scenario `ended`: the session is killed mid-run and its registry record
// goes with it.
//
//	t     registry   vigie
//	20s   idle       idle
//	30s   -          ended     ← the process is gone
//
// The record disappearing is not what decides it — a session the registry never
// covered must not read `ended` for that reason alone. What decides it is the
// process being confidently gone (#663), and this pins the two together.
func TestASessionWhoseProcessIsGoneReadsEnded(t *testing.T) {
	const id = "s-ended"
	f := newFleet(t)
	now := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)

	// A finished turn, old enough that no activity window holds it: what keeps the
	// session alive here is the registry record, and nothing else — which is the
	// point of dropping it next.
	f.writeFinishedTurnTranscript(id, 30*time.Minute, now)
	f.writeRegistry(id, "idle", "")
	f.scanAndReport(now)
	if got := f.statusOf(id); got == "ended" {
		t.Fatalf("the session reads ended while its record is there and its process alive")
	}

	f.dropRegistry(id)
	f.scanAndReport(now.Add(10 * time.Second))

	if got := f.statusOf(id); got != "ended" {
		t.Errorf("the board says %q after the session's process went, want ended", got)
	}
}

// dropRegistry removes the session's record, which is what a closed terminal
// leaves behind: Claude Code takes its record with it and tells vigie nothing.
func (f *fleet) dropRegistry(string) {
	f.t.Helper()
	dir := filepath.Join(f.home, ".claude", "sessions")
	if err := os.Remove(filepath.Join(dir, strconv.Itoa(os.Getpid())+".json")); err != nil {
		f.t.Fatal(err)
	}
}
