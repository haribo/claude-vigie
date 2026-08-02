package watch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeSession(t *testing.T, home, name, body string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSession(t, home, "1.json", `{"sessionId":"s1","status":"waiting","waitingFor":"Allow Bash?","pid":4242,"procStart":"98765","name":"my-conv","cwd":"/work","bridgeSessionId":"session_x"}`)
	writeSession(t, home, "2.json", `{"sessionId":"s2","status":"idle"}`) // sparse record
	writeSession(t, home, "skip.txt", `{"sessionId":"nope"}`)             // wrong extension
	writeSession(t, home, "bad.json", `not json`)                         // malformed

	m := readRegistry()
	if len(m) != 2 {
		t.Fatalf("readRegistry returned %d records, want 2", len(m))
	}
	r := m["s1"]
	if r.Status != "waiting" || r.WaitingFor != "Allow Bash?" {
		t.Errorf("s1 status fields = %+v", r)
	}
	if r.PID != 4242 || r.ProcStart != 98765 || r.BridgeSessionID != "session_x" {
		t.Errorf("s1 record = %+v", r)
	}
	if r.remoteURL() != "https://claude.ai/code/session_x" {
		t.Errorf("s1 remoteURL = %q", r.remoteURL())
	}
	if m["s2"].Status != "idle" || m["s2"].BridgeSessionID != "" || m["s2"].remoteURL() != "" {
		t.Errorf("s2 record = %+v", m["s2"])
	}
}

// selfProcStart reads this process's start time (field 22 of /proc/self/stat), so
// a test can write a registry record whose liveness check passes.
func selfProcStart(t *testing.T) uint64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skipf("no /proc: %v", err)
	}
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 20 {
		t.Fatalf("unexpected /proc/self/stat: %q", s)
	}
	v, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		t.Fatalf("parsing starttime: %v", err)
	}
	return v
}

func TestScanRegistryWaitingWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ps := selfProcStart(t)
	pid := os.Getpid()
	// A live session the registry reports as waiting on a permission.
	writeSession(t, home, "live.json",
		`{"sessionId":"s-reg","status":"waiting","waitingFor":"Allow Bash(git push)?","pid":`+
			strconv.Itoa(pid)+`,"procStart":"`+strconv.FormatUint(ps, 10)+`"}`)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	// The transcript's last turn stopped on a tool_use → the heuristic alone would
	// derive "working"; the registry's live "waiting" must win.
	writeLines(t, filepath.Join(proj, "s-reg.jsonl"), []string{
		`{"sessionId":"s-reg","cwd":"/work","type":"assistant","message":{"id":"m1","stop_reason":"tool_use","usage":{"output_tokens":5}}}`,
	})

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var got *struct{ status, activity string }
	for _, r := range reports {
		if r.SessionID == "s-reg" {
			got = &struct{ status, activity string }{r.Status, r.Activity}
		}
	}
	if got == nil {
		t.Fatal("no report for s-reg")
	}
	if got.status != "waiting" {
		t.Errorf("status = %q, want waiting (registry wins over the transcript's tool_use)", got.status)
	}
	if got.activity != "Allow Bash(git push)?" {
		t.Errorf("activity = %q, want the waitingFor reason", got.activity)
	}
}

func TestMapRegistryStatus(t *testing.T) {
	for in, want := range map[string]string{"busy": "working", "idle": "idle", "shell": "idle", "waiting": "waiting", "": "idle", "weird": "idle"} {
		if got := mapRegistryStatus(in); got != want {
			t.Errorf("mapRegistryStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanRegistryAliveBeatsStaleHeuristic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // no hook mapping here → the heuristic alone reads ended
	ps := selfProcStart(t)
	writeSession(t, home, "live.json",
		`{"sessionId":"s-busy","status":"busy","pid":`+strconv.Itoa(os.Getpid())+`,"procStart":"`+strconv.FormatUint(ps, 10)+`"}`)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	// An old dated line: with no hook mapping the transcript heuristic would call
	// this quiet session ended; the registry knows the process is alive and busy.
	writeLines(t, filepath.Join(proj, "s-busy.jsonl"), []string{
		`{"sessionId":"s-busy","timestamp":"2026-01-01T00:00:00Z","type":"assistant","message":{"id":"m","stop_reason":"end_turn","usage":{"output_tokens":1}}}`,
	})

	reports, err := Scan(root, "m", 100000*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, r := range reports {
		if r.SessionID == "s-busy" {
			if r.Status != "working" {
				t.Errorf("status = %q, want working (registry busy beats the stale heuristic)", r.Status)
			}
			return
		}
	}
	t.Fatal("no report for s-busy")
}

func TestScanRegistryDeadIsEnded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A registry record for a process that is not running (pid 2^30, wrong start).
	writeSession(t, home, "dead.json",
		`{"sessionId":"s-dead","status":"busy","pid":1073741824,"procStart":"1"}`)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	// A freshly-written transcript would derive "working"; the dead process wins.
	writeLines(t, filepath.Join(proj, "s-dead.jsonl"), []string{
		`{"sessionId":"s-dead","type":"assistant","message":{"id":"m","stop_reason":"end_turn","usage":{"output_tokens":1}}}`,
	})

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, r := range reports {
		if r.SessionID == "s-dead" {
			if r.Status != "ended" {
				t.Errorf("status = %q, want ended (backing process gone)", r.Status)
			}
			return
		}
	}
	t.Fatal("no report for s-dead")
}
