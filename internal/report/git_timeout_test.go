package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// #658. Every hook vigie installs stamps the session's git branch, and
// `PostToolUse` is installed by default — so this runs once per tool call,
// inside a hook the session waits on, against a budget vigie sets itself at 5 s.
//
// An unbounded `git` on that path is the class docs/design/transcript-reads.md
// exists to keep out of it: an `index.lock` held by another process or a repo on
// a stalled mount spends the whole budget on a field the function's own comment
// calls best-effort context. Claude Code kills the hook, and the report goes with
// it — the status transition, the heartbeat, the activity message.
//
// The fake `git` below never returns. What is asserted is only that gitBranch
// does, quickly, and empty-handed — the same answer it already gives for a
// directory that is not a repository.
func TestGitBranchGivesUpOnAHungGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake git is a shell script")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "git")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend rather than replace: the fake resolves first, and the script still
	// finds the `sleep` it runs. Replacing PATH outright makes it exit 127 at once
	// and the test measures nothing.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	done := make(chan string, 1)
	start := time.Now()
	go func() { done <- gitBranch(t.TempDir()) }()

	select {
	case got := <-done:
		if got != "" {
			t.Errorf("gitBranch = %q from a git that never answered, want empty", got)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("gitBranch took %s; a hook has 5 s in total for everything it does", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gitBranch never returned — it would have burned the whole hook budget and lost the report")
	}
}

// The bound must not cost the field itself: a healthy repository still names its
// branch, well inside the deadline.
func TestGitBranchStillNamesTheBranchOfARealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	// Fixed argument lists, no slice spread: gosec reads a variadic build-up as a
	// tainted subprocess invocation, and it is right to — there is no reason for
	// a fixture to construct its command dynamically.
	mustRun(t, exec.Command("git", "-C", dir, "init", "--initial-branch=trunk"))
	// `rev-parse HEAD` needs a commit: on an empty repository it fails and
	// gitBranch answers "" — pre-existing behavior, not what this pins.
	mustRun(t, exec.Command("git", "-C", dir,
		"-c", "user.name=t", "-c", "user.email=t@example.invalid",
		"commit", "--allow-empty", "-m", "root"))

	if got := gitBranch(dir); got != "trunk" {
		t.Errorf("gitBranch = %q, want trunk", got)
	}
	// A directory that is not a repository still answers empty, not an error.
	if got := gitBranch(t.TempDir()); got != "" {
		t.Errorf("gitBranch outside a repo = %q, want empty", got)
	}
}

func mustRun(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", cmd.Args, err, out)
	}
}
