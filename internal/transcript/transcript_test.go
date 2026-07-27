package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"ai-title","aiTitle":"auto title"}`,
		`{"sessionId":"sess-1","cwd":"/home/x/proj","gitBranch":"main","type":"user","timestamp":"2026-07-26T10:00:00Z","message":{"role":"user"}}`,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:01Z","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":10}}}`,
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":40}}}`, // duplicate id: usage ignored
		`{"type":"assistant","timestamp":"2026-07-26T10:00:05Z","message":{"id":"m2","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":200,"output_tokens":60,"cache_creation_input_tokens":5}}}`,
		`{"type":"custom-title","customTitle":"my-session","timestamp":"2026-07-26T10:00:06Z"}`,
		`not json`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", info.SessionID)
	}
	if info.Cwd != "/home/x/proj" {
		t.Errorf("Cwd = %q", info.Cwd)
	}
	if info.GitBranch != "main" {
		t.Errorf("GitBranch = %q", info.GitBranch)
	}
	if info.Title != "my-session" {
		t.Errorf("Title = %q, want my-session (customTitle over aiTitle)", info.Title)
	}
	if info.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q", info.Model)
	}
	if info.Usage.InputTokens != 300 || info.Usage.OutputTokens != 100 ||
		info.Usage.CacheReadTokens != 10 || info.Usage.CacheCreationTokens != 5 {
		t.Errorf("Usage = %+v, want in=300 out=100 cacheRead=10 cacheCreate=5", info.Usage)
	}
	if info.LastStopReason != "end_turn" {
		t.Errorf("LastStopReason = %q, want end_turn", info.LastStopReason)
	}
	if info.LastActivity != "2026-07-26T10:00:06Z" {
		t.Errorf("LastActivity = %q, want the last timestamp", info.LastActivity)
	}
}

func TestParseInFlightTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		// Two background shells launched.
		`{"type":"user","message":{"content":"Command running in background with ID: shellA"}}`,
		`{"type":"user","message":{"content":"Command running in background with ID: shellB"}}`,
		// One of them completes via a task-notification carrying the same id.
		`{"type":"user","message":{"content":"<task-notification> <task-id>shellA</task-id> <output-file>/tmp/tasks/shellA.output</output-file> <status>completed</status> </task-notification>"}}`,
		// An unrelated agent task completes: its id was never launched as a shell,
		// so it must not affect the in-flight count.
		`{"type":"user","message":{"content":"<task-notification> <task-id>agentZ</task-id> <status>killed</status> </task-notification>"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.InFlightTasks != 1 {
		t.Errorf("InFlightTasks = %d, want 1 (shellB still running)", info.InFlightTasks)
	}
}
