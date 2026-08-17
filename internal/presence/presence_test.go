package presence

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, ok, err := Load("sess-1"); err != nil || ok {
		t.Fatalf("Load before save: ok=%v err=%v, want ok=false nil", ok, err)
	}

	want := Mapping{PID: 4242, StartTime: 99999}
	if err := Save("sess-1", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := Load("sess-1")
	if err != nil || !ok {
		t.Fatalf("Load after save: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}

	if err := Delete("sess-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := Load("sess-1"); ok {
		t.Error("Load after delete: ok=true, want false")
	}
	if err := Delete("sess-1"); err != nil {
		t.Errorf("Delete when absent: %v, want nil", err)
	}
}

func TestGC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	self := os.Getpid()
	_, _, start, err := readStat(self)
	if err != nil {
		t.Skipf("no /proc: %v", err)
	}

	if err := Save("live", Mapping{PID: self, StartTime: start}); err != nil {
		t.Fatal(err)
	}
	if err := Save("dead-old", Mapping{PID: 2 << 30, StartTime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Save("dead-new", Mapping{PID: 2 << 30, StartTime: 1}); err != nil {
		t.Fatal(err)
	}
	// Age the dead-old mapping file beyond the threshold.
	p, _ := pathFor("dead-old")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	n, err := GC(24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if n != 1 {
		t.Errorf("removed %d, want 1 (only dead+old)", n)
	}
	if _, ok, _ := Load("dead-old"); ok {
		t.Error("dead-old should have been collected")
	}
	if _, ok, _ := Load("dead-new"); !ok {
		t.Error("dead-new should be kept (recently closed)")
	}
	if _, ok, _ := Load("live"); !ok {
		t.Error("live should be kept (process alive)")
	}
}

func TestPathForRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "../x"} {
		if _, err := pathFor(bad); err == nil {
			t.Errorf("pathFor(%q) = nil error, want rejection", bad)
		}
	}
}

func TestReadStatAndAlive(t *testing.T) {
	self := os.Getpid()
	comm, ppid, start, err := readStat(self)
	if err != nil {
		t.Fatalf("readStat(self): %v", err)
	}
	if comm == "" || ppid <= 0 || start == 0 {
		t.Fatalf("readStat(self) = comm=%q ppid=%d start=%d, want all set", comm, ppid, start)
	}

	if !Alive(Mapping{PID: self, StartTime: start}) {
		t.Error("Alive(self with correct start_time) = false, want true")
	}
	// A mismatched start time simulates a reused pid → must read as not alive.
	if Alive(Mapping{PID: self, StartTime: start + 1}) {
		t.Error("Alive(self with wrong start_time) = true, want false (pid-reuse guard)")
	}
	// A pid that almost certainly does not exist.
	if Alive(Mapping{PID: 2 << 30, StartTime: 1}) {
		t.Error("Alive(nonexistent pid) = true, want false")
	}
}

// TestWatcherRunning starts a real child process that impersonates a watcher — a
// copy of /bin/sh named like our binary, invoked with a bare "watch" arg — and
// checks the /proc scan detects it, ignores self, and rejects a name mismatch (#371).
func TestWatcherRunning(t *testing.T) {
	if _, err := os.Stat("/proc"); err != nil {
		t.Skipf("no /proc: %v", err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh: %v", err)
	}
	// A copy of sh named for this run so its /proc comm matches the name we scan
	// for. The name is unique per process because the assertion below claims that
	// *nothing else on the machine* carries it — true on a developer's laptop,
	// not something a test can promise on a shared runner, which is where it
	// failed (#476). comm is truncated at 15 bytes, so the suffix fits in five.
	name := fmt.Sprintf("vigiepr%05d", os.Getpid()%100000)
	bin := filepath.Join(t.TempDir(), name)
	data, err := os.ReadFile(sh) //nolint:gosec // test-only copy of the system shell
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, data, 0o700); err != nil { //nolint:gosec // test binary must be executable
		t.Fatal(err)
	}
	// `read` is a POSIX builtin, so the shell blocks in itself and forks nothing
	// as long as its stdin stays open. That matters more than it looks: the
	// previous probe was `sh -c "sleep 300; :"`, which forks — and between that
	// fork and its execve the child carries the parent's comm *and* the parent's
	// cmdline, because the kernel copies both and only execve replaces them. For
	// a few microseconds there was therefore a process that was not the probe's
	// pid, was named like the probe, and had "watch" in its cmdline: exactly
	// WatcherRunning's predicate, which is what the assertion below caught at
	// random on CI (#476).
	//
	// The bare "watch" ($0) lands in the shell's own argv for the scan to find.
	cmd := exec.Command(bin, "-c", "read line", "watch") //nolint:gosec // fixed test args
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting probe: %v", err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	// Poll briefly: the child needs a moment to appear in /proc under its comm.
	found := false
	for i := 0; i < 50; i++ {
		if ok, err := WatcherRunning(name, os.Getpid()); err != nil {
			t.Fatalf("WatcherRunning: %v", err)
		} else if ok {
			found = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Error("WatcherRunning did not detect the running probe")
	}
	// The guard for the guard, checked once the probe is established: a probe that
	// forks re-opens the window this test was failing in, and nothing else here
	// would notice.
	if kids := childrenOf(cmd.Process.Pid); len(kids) > 0 {
		t.Errorf("the probe forked %v — a child carries its comm and cmdline until it execs, and impersonates a watcher", kids)
	}
	// Excluding the probe's own pid hides it: nothing else carries this run's name.
	if ok, err := WatcherRunning(name, cmd.Process.Pid); err != nil || ok {
		t.Errorf("WatcherRunning(selfPID=probe) = %v, %v; want false, nil", ok, err)
	}
	// A name that no process carries is never a match.
	if ok, err := WatcherRunning("no-such-binxyz", os.Getpid()); err != nil || ok {
		t.Errorf("WatcherRunning(unknown) = %v, %v; want false, nil", ok, err)
	}
}

// childrenOf lists the pids whose parent is pid, read from /proc.
func childrenOf(pid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		child, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat")) //nolint:gosec // a /proc path we built
		if err != nil {
			continue
		}
		// comm may contain spaces and parentheses, so the numeric fields start
		// after the final ')': ppid is the second of them.
		rest := string(data)
		if i := strings.LastIndex(rest, ")"); i >= 0 {
			fields := strings.Fields(rest[i+1:])
			if len(fields) >= 2 && fields[1] == strconv.Itoa(pid) {
				out = append(out, child)
			}
		}
	}
	return out
}
