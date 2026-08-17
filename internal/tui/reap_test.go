package tui

import (
	"os/exec"
	"testing"
	"time"
)

// #560. The desktop notifier called `Start` and nothing else, so every child it
// spawned stayed a zombie until the TUI exited. `Run` is not the fix — it blocks,
// and this runs on the update path — so the child is waited on in a goroutine.
//
// The assertion is `ProcessState`, which `Wait` is what sets: reading it without
// synchronizing with that goroutine would be a data race, and CI runs with
// -race. Hence the `done` callback, whose whole purpose is to make the reap
// observable exactly once.
func TestTheNotifierReapsItsChild(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	done := make(chan struct{})
	startAndReap(cmd, func() { close(done) })

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the child was never waited on — it stays a zombie until the TUI exits")
	}
	if cmd.ProcessState == nil {
		t.Error("ProcessState is nil after the reap signaled — Wait was not called")
	}
}

// A command that cannot start must not leave the caller hanging on a signal that
// never comes, or a future caller that waits on `done` deadlocks.
func TestAChildThatCannotStartStillSignals(t *testing.T) {
	cmd := exec.Command("/nonexistent/definitely-not-a-binary")
	done := make(chan struct{})
	startAndReap(cmd, func() { close(done) })

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a failed start never signaled")
	}
}

// Production passes nil, and that must not panic.
func TestReapingWithoutASignalIsFine(t *testing.T) {
	startAndReap(exec.Command("/bin/sh", "-c", "exit 0"), nil)
}
