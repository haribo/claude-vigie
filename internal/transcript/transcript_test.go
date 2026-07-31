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

func TestParseAPIError(t *testing.T) {
	dir := t.TempDir()

	// A trailing API-error line surfaces its HTTP status.
	errored := filepath.Join(dir, "err.jsonl")
	writeJSONL(t, errored,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:00Z","message":{"id":"m1","stop_reason":"end_turn","usage":{"output_tokens":5}}}`,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:02Z","isApiErrorMessage":true,"apiErrorStatus":529,"message":{}}`,
	)
	if info, err := Parse(errored); err != nil {
		t.Fatal(err)
	} else if info.LastAPIError != 529 {
		t.Errorf("LastAPIError = %d, want 529", info.LastAPIError)
	}

	// A later non-error assistant line clears it (transient/ephemeral).
	recovered := filepath.Join(dir, "ok.jsonl")
	writeJSONL(t, recovered,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:02Z","isApiErrorMessage":true,"apiErrorStatus":500,"message":{}}`,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:04Z","message":{"id":"m2","stop_reason":"end_turn"}}`,
	)
	if info, err := Parse(recovered); err != nil {
		t.Fatal(err)
	} else if info.LastAPIError != 0 {
		t.Errorf("LastAPIError = %d, want 0 (cleared by a later ok line)", info.LastAPIError)
	}
}

func TestParseThinking(t *testing.T) {
	dir := t.TempDir()

	// Last assistant block is a thinking block → thinking.
	thinking := filepath.Join(dir, "think.jsonl")
	writeJSONL(t, thinking,
		`{"type":"assistant","timestamp":"2026-07-26T10:00:00Z","message":{"id":"m1","content":[{"type":"thinking","thinking":"…"}]}}`,
	)
	if info, err := Parse(thinking); err != nil {
		t.Fatal(err)
	} else if !info.Thinking {
		t.Error("Thinking = false, want true (last block is thinking)")
	}

	// A later text block clears it (thinking is transient).
	produced := filepath.Join(dir, "text.jsonl")
	writeJSONL(t, produced,
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"thinking","thinking":"…"}]}}`,
		`{"type":"assistant","message":{"id":"m2","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"…"},{"type":"text","text":"done"}]}}`,
	)
	if info, err := Parse(produced); err != nil {
		t.Fatal(err)
	} else if info.Thinking {
		t.Error("Thinking = true, want false (last block is text)")
	}
}

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
