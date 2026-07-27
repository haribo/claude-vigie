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

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		stop string
		age  time.Duration
		want string
	}{
		{"tool_use", time.Hour, "working"},       // tool_use → working regardless of age
		{"end_turn", 3 * time.Second, "working"}, // very recent activity → working
		{"end_turn", 2 * time.Minute, "waiting"},
		{"end_turn", time.Hour, "idle"},
	}
	for _, c := range cases {
		if got := deriveStatus(c.stop, c.age); got != c.want {
			t.Errorf("deriveStatus(%q, %s) = %q, want %q", c.stop, c.age, got, c.want)
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
	if got := statusFor("live", "tool_use", time.Hour); got != "working" {
		t.Errorf("alive + tool_use = %q, want working", got)
	}

	// A dead mapping = a pid that does not exist → ended.
	if err := presence.Save("dead", presence.Mapping{PID: 2 << 30, StartTime: 1}); err != nil {
		t.Fatal(err)
	}
	if got := statusFor("dead", "end_turn", time.Hour); got != "ended" {
		t.Errorf("dead process = %q, want ended", got)
	}

	// No mapping → activity fallback (old transcript → idle).
	if got := statusFor("unmapped", "end_turn", time.Hour); got != "idle" {
		t.Errorf("no mapping + old = %q, want idle (fallback)", got)
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
