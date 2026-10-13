package presence

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The macOS reader (ADR-0018, #869). The Linux tests in this package exercise /proc,
// faked or real; these exercise sysctl on the real process table, because the
// properties that matter — the start time agrees with Claude Code's registry, a
// reaped pid reads Gone rather than Unknown — are properties of the kernel.

// helperEnv switches the test binary into one of the helper roles below, so a
// test can stand up a process whose argv[0] it chooses without copying a system
// binary (which on macOS is signed, and /bin/sh re-execs another shell).
const helperEnv = "VIGIE_PRESENCE_HELPER"

// TestHelperProcess is not a test: it is the body of the processes the tests
// below start. Under `go test` without the variable it returns at once.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv(helperEnv) {
	case "":
		return
	case "block":
		// Stays alive, forking nothing, until the parent closes stdin.
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	case "claude":
		// Stands in for Claude Code: starts a "hook" below itself and relays its answer.
		hook, err := helperCmd("resolve", "/opt/example/hook")
		if err == nil {
			var out []byte
			out, err = hook.Output()
			fmt.Print(string(out))
		}
		if err != nil {
			fmt.Printf("hook failed: %v\n", err)
		}
	case "resolve":
		m, err := ResolveClaude()
		fmt.Printf("%d %d %v\n", m.PID, m.StartTime, err)
	}
	os.Exit(0)
}

// helperCmd runs the test binary in a helper role under the given argv[0]. The
// binary is located through os.Executable, never os.Args[0]: inside a helper,
// argv[0] is whatever the test chose — "claude" resolves through PATH to the real
// Claude Code.
func helperCmd(role, argv0 string, extra ...string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, append([]string{"-test.run=^TestHelperProcess$"}, extra...)...) //nolint:gosec // the test binary itself
	cmd.Args[0] = argv0
	cmd.Env = append(os.Environ(), helperEnv+"="+role)
	return cmd, nil
}

func helper(t *testing.T, role, argv0 string, extra ...string) *exec.Cmd {
	t.Helper()
	cmd, err := helperCmd(role, argv0, extra...)
	if err != nil {
		t.Fatal(err)
	}
	return cmd
}

func TestInspectSelf(t *testing.T) {
	p, err := inspect(os.Getpid())
	if err != nil {
		t.Fatalf("inspect(self): %v", err)
	}
	if p.ppid != os.Getppid() {
		t.Errorf("ppid = %d, want %d", p.ppid, os.Getppid())
	}
	if want := filepath.Base(os.Args[0]); p.name != want {
		t.Errorf("name = %q, want argv[0]'s base %q", p.name, want)
	}
	// A start time is seconds since the epoch, and this process began recently.
	if age := time.Since(time.Unix(int64(p.start), 0)); age < 0 || age > time.Hour {
		t.Errorf("start = %d (%v ago), not this process's start", p.start, age)
	}
}

// The pid-reuse guard only works if the two sides agree: the start time vigie
// reads must equal the one Claude Code writes into its registry. The string is a
// recorded sample (ADR-0016), from a session whose kinfo_proc start time was
// read at the same moment: 2026-10-12 09:56:18 UTC.
func TestParseProcStartMatchesTheRegistry(t *testing.T) {
	want := uint64(time.Date(2026, 10, 12, 9, 56, 18, 0, time.UTC).Unix())
	if got := ParseProcStart("Mon Oct 12 09:56:18 2026"); got != want {
		t.Errorf("recorded sample = %d, want %d", got, want)
	}
	// Single-digit days: not yet observed from Claude Code, so both C padding styles.
	day5 := uint64(time.Date(2026, 10, 5, 9, 56, 18, 0, time.UTC).Unix())
	for _, raw := range []string{"Mon Oct  5 09:56:18 2026", "Mon Oct 05 09:56:18 2026"} {
		if got := ParseProcStart(raw); got != day5 {
			t.Errorf("%q = %d, want %d", raw, got, day5)
		}
	}
	// Anything else carries no guard rather than a wrong one.
	for _, raw := range []string{"", "123456", "yesterday"} {
		if got := ParseProcStart(raw); got != 0 {
			t.Errorf("%q = %d, want 0", raw, got)
		}
	}
	// And the reader agrees with the format, on a live process.
	p, err := inspect(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	raw := time.Unix(int64(p.start), 0).UTC().Format(time.ANSIC)
	if got := ParseProcStart(raw); got != p.start {
		t.Errorf("own start %q parsed to %d, inspect said %d", raw, got, p.start)
	}
}

// A pid with no process must read Gone, not Unknown: sysctl does not answer
// ENOENT for it, and treating its answer as "could not look" would leave every
// crashed session alive forever — the mirror image of #663.
func TestStatusOfAReapedProcessIsGone(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if got := Status(Mapping{PID: pid, StartTime: 1}); got != Gone {
		t.Errorf("reaped pid %d → %v, want Gone", pid, got)
	}

	p, err := inspect(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if got := Status(Mapping{PID: os.Getpid(), StartTime: p.start}); got != Live {
		t.Errorf("self → %v, want Live", got)
	}
	if got := Status(Mapping{PID: os.Getpid(), StartTime: p.start + 1}); got != Gone {
		t.Errorf("self with another start time → %v, want Gone (pid-reuse guard)", got)
	}
}

// On macOS the Claude Code process's comm is its version file ("2.1.296"), not
// "claude" — only argv[0] carries the name. A hook must still find it.
func TestResolveClaudeFindsTheAncestorByArgv0(t *testing.T) {
	cmd := helper(t, "claude", "claude")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v", err)
	}
	var pid int
	var start uint64
	var errText string
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d %s", &pid, &start, &errText); err != nil {
		t.Fatalf("helper said %q: %v", out, err)
	}
	if errText != "<nil>" {
		t.Fatalf("ResolveClaude under a claude parent: %q", out)
	}
	if pid != cmd.Process.Pid {
		t.Errorf("resolved pid %d, want the claude stand-in %d", pid, cmd.Process.Pid)
	}
	if start == 0 {
		t.Error("resolved a zero start time")
	}
}

func TestWatcherRunningByArgv0(t *testing.T) {
	// Unique per run: the assertion claims nothing else on the machine carries it (#476).
	name := "vigiepr" + strconv.Itoa(os.Getpid())
	cmd := helper(t, "block", filepath.Join("/opt/example", name), "watch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _, _ = cmd.Process.Wait() })

	found := false
	for i := 0; i < 50 && !found; i++ {
		ok, err := WatcherRunning(name, os.Getpid())
		if err != nil {
			t.Fatalf("WatcherRunning: %v", err)
		}
		found = ok
		if !found {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !found {
		t.Error("WatcherRunning did not detect the running probe")
	}
	if ok, err := WatcherRunning(name, cmd.Process.Pid); err != nil || ok {
		t.Errorf("WatcherRunning(selfPID=probe) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := WatcherRunning("no-such-binxyz", os.Getpid()); err != nil || ok {
		t.Errorf("WatcherRunning(unknown) = %v, %v; want false, nil", ok, err)
	}
}
