package presence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #663. `Alive` answered false for every failure to read /proc, so a hardened
// `hidepid`, a container or a namespace that does not expose the pid read exactly
// like a dead process — and `registryDead`, which takes its pid straight from
// Claude Code's registry, then reported every session in the fleet `ended` on the
// next scan.
//
// The three answers are what the callers needed: absent, present, and "I could
// not look". Only the first two are evidence.

// fakeProc points the reader at a temporary process table and returns it.
func fakeProc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := procRoot
	procRoot = dir
	t.Cleanup(func() { procRoot = orig })
	return dir
}

func writeStat(t *testing.T, dir string, pid int, body string) {
	t.Helper()
	pd := filepath.Join(dir, itoa(pid))
	if err := os.MkdirAll(pd, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pd, "stat"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// statLine builds a /proc/<pid>/stat line of the shape readStat expects. After
// the closing paren the fields are [0]=state, [1]=ppid, and [19]=starttime — so
// seventeen fillers sit between the ppid and the start time, not eighteen. The
// first version of this helper miscounted by one and the test failed on the
// *Live* case, which is the useful way for a fixture to be wrong.
func statLine(ppid int, starttime uint64) string {
	s := "42 (claude) S " + itoa(ppid)
	for i := 0; i < 17; i++ {
		s += " 0"
	}
	return s + " " + itoa(int(starttime)) + " 0 0\n"
}

func TestStatusSeparatesGoneFromUnreadable(t *testing.T) {
	dir := fakeProc(t)

	// Nothing at all: the process is not there.
	if got := Status(Mapping{PID: 42, StartTime: 100}); got != Gone {
		t.Errorf("no entry → %v, want Gone", got)
	}

	// Present with the recorded start time.
	writeStat(t, dir, 42, statLine(1, 100))
	if got := Status(Mapping{PID: 42, StartTime: 100}); got != Live {
		t.Errorf("matching start time → %v, want Live", got)
	}

	// Present with a different start time: the pid was reused, ours is gone.
	if got := Status(Mapping{PID: 42, StartTime: 999}); got != Gone {
		t.Errorf("reused pid → %v, want Gone", got)
	}

	// There, but unreadable — the case that used to read as death. A malformed
	// file stands for every reason /proc can refuse: hidepid, a namespace, a
	// permission error.
	writeStat(t, dir, 42, "not a stat file\n")
	if got := Status(Mapping{PID: 42, StartTime: 100}); got != Unknown {
		t.Errorf("unreadable → %v, want Unknown — this is what declared a whole fleet ended", got)
	}
	if Alive(Mapping{PID: 42, StartTime: 100}) {
		t.Error("Alive said true for an unreadable process")
	}
}

// The garbage collector removes mappings whose process is gone. A mapping it
// could not read is not one of those: deleting it would remove the evidence that
// the session is alive, and the watcher derives `ended` from a missing mapping —
// so the collector would produce exactly the verdict #663 is about, one step
// further along.
func TestGCKeepsAMappingItCannotVerify(t *testing.T) {
	dir := fakeProc(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := Save("s1", Mapping{PID: 42, StartTime: 100}); err != nil {
		t.Fatal(err)
	}
	// There, but unreadable.
	writeStat(t, dir, 42, "not a stat file\n")

	// Well past any age threshold, so only the liveness answer decides.
	n, err := GC(0, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("collected %d unverifiable mapping(s), want 0", n)
	}
	if _, ok, err := Load("s1"); err != nil || !ok {
		t.Errorf("the mapping was deleted (ok=%v, err=%v)", ok, err)
	}

	// And one whose process is genuinely absent is still collected.
	if err := os.RemoveAll(filepath.Join(dir, "42")); err != nil {
		t.Fatal(err)
	}
	if n, err = GC(0, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Errorf("collected %d for an absent process (err=%v), want 1", n, err)
	}
}
