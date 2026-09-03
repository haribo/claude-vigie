package presence

import (
	"os"
	"path/filepath"
	"testing"
)

// ADR-0013 decision 3 (#714). To know whether a session is alive, vigie needs the
// Claude Code pid. It went looking for it — up to twenty ancestors, comparing each
// process name against "claude", around the kernel's 15-byte truncation of `comm`
// — when the hook is handed that pid directly:
//
//	CLAUDE_PID=157048
//
// Measured on a live session, and equal to what the walk finds.
//
// The walk's real cost is not its complexity, it is how it fails. It succeeds only
// while the hook is a descendant of a process literally named `claude`; a wrapper,
// a shim or a re-exec breaks it, and then nothing is written at all — the session
// gets no mapping and liveness falls back to the transcript heuristic, which is
// the path #660 came from.
//
// `ResolveClaude` had no test before this.

// claudeStat is a /proc/<pid>/stat line for a process named `claude`.
func claudeStat(starttime uint64) string { return statLine(1, starttime) }

// namedStat is one for a process called something else.
func namedStat(comm string, starttime uint64) string {
	s := "7 (" + comm + ") S 1"
	for i := 0; i < 17; i++ {
		s += " 0"
	}
	return s + " " + itoa(int(starttime)) + " 0 0\n"
}

func TestResolveClaudeUsesThePidItIsGiven(t *testing.T) {
	dir := fakeProc(t)
	// Nothing here can be reached by walking: the test process is not in this
	// table, so a mapping can only come from the environment.
	writeStat(t, dir, 999, claudeStat(4242))
	t.Setenv(claudePIDEnv, "999")

	m, err := ResolveClaude()
	if err != nil {
		t.Fatalf("ResolveClaude: %v — the pid was handed over and still not used", err)
	}
	if m.PID != 999 || m.StartTime != 4242 {
		t.Errorf("mapping = %+v, want {999 4242}", m)
	}
}

// The start time still comes from /proc. The mapping is {pid, procStart} so that
// pid reuse cannot resurrect a dead session (ADR-0006); reading the pid from the
// environment does not change what makes it safe.
func TestResolveClaudeStillReadsTheStartTime(t *testing.T) {
	dir := fakeProc(t)
	writeStat(t, dir, 999, claudeStat(777))
	t.Setenv(claudePIDEnv, "999")

	m, err := ResolveClaude()
	if err != nil {
		t.Fatal(err)
	}
	if m.StartTime != 777 {
		t.Errorf("StartTime = %d, want 777 — a pid alone does not identify a process", m.StartTime)
	}
}

// The fallback is the point of the decision: a variable present today and absent
// tomorrow must not take the walk down with it.
func TestResolveClaudeFallsBackWhenTheEnvIsUnusable(t *testing.T) {
	dir := fakeProc(t)
	writeStat(t, dir, 999, claudeStat(4242))

	cases := map[string]string{
		"unset":                  "",
		"not a number":           "not-a-pid",
		"zero":                   "0",
		"a process that is gone": "12345",
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(claudePIDEnv, v)
			// The walk cannot succeed in this table either, so an error *is* the
			// fallback running: what must not happen is a mapping invented from an
			// unusable variable.
			if m, err := ResolveClaude(); err == nil {
				t.Errorf("returned %+v from %s=%q instead of falling back", m, claudePIDEnv, v)
			}
		})
	}
}

// A pid that names something else is not Claude Code. Trusting it would map the
// session to a process it does not run — and the session would read `ended` the
// moment that process exits.
func TestResolveClaudeRefusesAPidThatIsNotClaude(t *testing.T) {
	dir := fakeProc(t)
	writeStat(t, dir, 7, namedStat("bash", 1))
	t.Setenv(claudePIDEnv, "7")

	if m, err := ResolveClaude(); err == nil {
		t.Errorf("returned %+v for a process named bash", m)
	}
}

// Guard on the fixture itself: the fake table is what these tests rest on, so a
// silently empty one would make every case above pass for the wrong reason.
func TestTheFakeProcTableIsReadable(t *testing.T) {
	dir := fakeProc(t)
	writeStat(t, dir, 999, claudeStat(4242))
	if _, err := os.Stat(filepath.Join(dir, "999", "stat")); err != nil {
		t.Fatalf("the fixture wrote nothing: %v", err)
	}
	comm, _, start, err := readStat(999)
	if err != nil || comm != "claude" || start != 4242 {
		t.Fatalf("readStat = (%q, _, %d, %v); the fixture does not parse", comm, start, err)
	}
}
