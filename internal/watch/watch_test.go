package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		stop     string
		inFlight int
		age      time.Duration
		want     string
	}{
		{"tool_use", 0, time.Hour, "working"},       // tool_use → working regardless of age
		{"end_turn", 0, 3 * time.Second, "working"}, // very recent activity → working
		{"end_turn", 2, time.Hour, "waiting"},       // finished but background tasks in flight
		{"end_turn", 0, time.Hour, "idle"},          // finished, nothing pending
		{"tool_use", 3, time.Hour, "working"},       // active work outranks in-flight count
	}
	for _, c := range cases {
		if got := deriveStatus(c.stop, c.inFlight, c.age); got != c.want {
			t.Errorf("deriveStatus(%q, %d, %s) = %q, want %q", c.stop, c.inFlight, c.age, got, c.want)
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
		`{"type":"user","message":{"content":"Command running in background with ID: bg42"}}`,
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
