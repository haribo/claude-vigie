package watch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-fleet/internal/presence"
)

func TestActivelyWorking(t *testing.T) {
	cases := []struct {
		stop string
		age  time.Duration
		want bool
	}{
		{"end_turn", 3 * time.Second, true}, // very recent write
		{"end_turn", time.Hour, false},      // idle between turns
		{"tool_use", 2 * time.Minute, true}, // tool running, within toolWindow
		{"tool_use", time.Hour, false},      // stuck/old tool, past toolWindow
	}
	for _, c := range cases {
		if got := activelyWorking(c.stop, c.age); got != c.want {
			t.Errorf("activelyWorking(%q, %s) = %v, want %v", c.stop, c.age, got, c.want)
		}
	}
}

func TestScanFiltersOldAndBuildsReports(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj-a")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}

	recent := filepath.Join(proj, "sess-recent.jsonl")
	writeLines(t, recent, []string{
		`{"sessionId":"s-recent","cwd":"/home/x/a","gitBranch":"main","type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"output_tokens":100}}}`,
	})

	old := filepath.Join(proj, "sess-old.jsonl")
	writeLines(t, old, []string{
		`{"sessionId":"s-old","cwd":"/home/x/b","type":"assistant","message":{"id":"m1","stop_reason":"end_turn"}}`,
	})
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	reports, err := Scan(root, "laptop", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("len = %d, want 1 (old session filtered out)", len(reports))
	}
	r := reports[0]
	if r.SessionID != "s-recent" {
		t.Errorf("SessionID = %q", r.SessionID)
	}
	if r.Machine != "laptop" || r.ProjectDir != "/home/x/a" || r.GitBranch != "main" {
		t.Errorf("context wrong: %+v", r)
	}
	if r.Event != "watch" {
		t.Errorf("Event = %q, want watch", r.Event)
	}
	if r.Status == "" {
		t.Error("Status is empty")
	}
	if r.Usage == nil || r.Usage.OutputTokens != 100 {
		t.Errorf("Usage = %+v, want output 100", r.Usage)
	}
}

func TestScanTimestampFromLastActivityNotMtime(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no presence mapping → old activity reads ended
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	lastActivity := now.Add(-3 * time.Hour) // last real line, hours ago
	f := filepath.Join(proj, "s.jsonl")
	writeLines(t, f, []string{
		`{"sessionId":"s1","type":"assistant","timestamp":"` + lastActivity.UTC().Format(time.RFC3339) +
			`","message":{"id":"m1","stop_reason":"end_turn","usage":{"output_tokens":5}}}`,
	})
	// Simulate the hourly metadata churn: the file mtime is now, though no dated
	// line was appended.
	if err := os.Chtimes(f, now, now); err != nil {
		t.Fatal(err)
	}

	reports, err := Scan(root, "m", 24*time.Hour, now)
	if err != nil || len(reports) != 1 {
		t.Fatalf("Scan = %+v (err %v), want 1 report", reports, err)
	}
	r := reports[0]
	// SEEN must reflect the last dated line, not the recent mtime.
	if want := lastActivity.UTC().Format(time.RFC3339); r.Timestamp != want {
		t.Errorf("Timestamp = %q, want %q (last activity, not mtime)", r.Timestamp, want)
	}
	// A recent mtime must not flash the session as working: the age used for the
	// status derivation comes from the last activity, which is hours old.
	if r.Status == "working" {
		t.Errorf("Status = working; a stale session must not read as working on a mtime bump")
	}
}

func TestScannerCachesParse(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "p")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(proj, "s.jsonl")
	writeLines(t, f, []string{
		`{"sessionId":"s1","type":"assistant","message":{"id":"m","stop_reason":"end_turn","usage":{"output_tokens":10}}}`,
	})
	fi, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}

	sc := newScanner()
	r1, err := sc.scan(root, "m", time.Hour, time.Now())
	if err != nil || len(r1) != 1 || r1[0].Usage.OutputTokens != 10 {
		t.Fatalf("first scan: %+v (err %v)", r1, err)
	}
	if len(sc.cache) != 1 {
		t.Fatalf("cache not populated: len=%d", len(sc.cache))
	}

	// Rewrite the file with different (invalid) content of the SAME size, then
	// restore its mod time. A re-parse would yield 0 tokens; a cache hit keeps 10.
	if err := os.WriteFile(f, []byte(strings.Repeat("x", int(fi.Size()))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	r2, err := sc.scan(root, "m", time.Hour, time.Now())
	if err != nil || len(r2) != 1 || r2[0].Usage.OutputTokens != 10 {
		t.Errorf("cached scan should reuse the parse (10 tokens), got %+v (err %v)", r2, err)
	}
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStatusForPresence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// A live mapping = the test process itself (present, matching start time).
	if err := presence.Save("live", presence.Mapping{PID: os.Getpid(), StartTime: selfStartTime(t)}); err != nil {
		t.Fatal(err)
	}
	if got := statusFor("live", "end_turn", time.Hour); got != "idle" {
		t.Errorf("alive + old + end_turn = %q, want idle (live sessions never go stale)", got)
	}
	if got := statusFor("live", "tool_use", 2*time.Minute); got != "working" {
		t.Errorf("alive + tool_use in window = %q, want working", got)
	}
	if got := statusFor("live", "tool_use", time.Hour); got != "idle" {
		t.Errorf("alive + stuck tool = %q, want idle (tool_use is bounded)", got)
	}

	// A dead mapping = a pid that does not exist → ended.
	if err := presence.Save("dead", presence.Mapping{PID: 2 << 30, StartTime: 1}); err != nil {
		t.Fatal(err)
	}
	if got := statusFor("dead", "end_turn", time.Hour); got != "ended" {
		t.Errorf("dead process = %q, want ended", got)
	}

	// No mapping + inactive → ended (presumed closed).
	if got := statusFor("unmapped", "end_turn", time.Hour); got != "ended" {
		t.Errorf("no mapping + inactive = %q, want ended", got)
	}
	// No mapping but actively writing → working (not yet backfilled).
	if got := statusFor("unmapped", "end_turn", 2*time.Second); got != "working" {
		t.Errorf("no mapping + active = %q, want working", got)
	}
}

func TestSessionStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// No mapping + actively writing → working; an API error overrides to error.
	if got := sessionStatus("u1", "end_turn", 0, 2*time.Second); got != "working" {
		t.Errorf("active, no error = %q, want working", got)
	}
	if got := sessionStatus("u1", "end_turn", 500, 2*time.Second); got != "error" {
		t.Errorf("active + api error = %q, want error", got)
	}

	// A live but idle session with a lingering API error still shows error.
	if err := presence.Save("live-e", presence.Mapping{PID: os.Getpid(), StartTime: selfStartTime(t)}); err != nil {
		t.Fatal(err)
	}
	if got := sessionStatus("live-e", "end_turn", 529, time.Hour); got != "error" {
		t.Errorf("idle + api error = %q, want error", got)
	}

	// A closed/ended session is never shown as error, even if its last line errored.
	if got := sessionStatus("u2", "end_turn", 500, time.Hour); got != "ended" {
		t.Errorf("ended + api error = %q, want ended (not sticky-red on a closed session)", got)
	}
}

func TestWithThinking(t *testing.T) {
	cases := []struct {
		status   string
		thinking bool
		want     string
	}{
		{"working", true, "thinking"},
		{"idle", true, "thinking"},
		{"working", false, "working"},
		{"idle", false, "idle"},
		{"error", true, "error"},     // never overrides an API error
		{"ended", true, "ended"},     // never overrides a closed session
		{"waiting", true, "waiting"}, // never overrides waiting-on-human
	}
	for _, c := range cases {
		if got := withThinking(c.status, c.thinking); got != c.want {
			t.Errorf("withThinking(%q, %v) = %q, want %q", c.status, c.thinking, got, c.want)
		}
	}
}

// selfStartTime reads field 22 of /proc/self/stat (the test process start time).
func selfStartTime(t *testing.T) uint64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skipf("no /proc (non-Linux?): %v", err)
	}
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	v, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		t.Fatalf("parsing start time: %v", err)
	}
	return v
}
