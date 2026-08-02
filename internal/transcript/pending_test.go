package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseLines(t *testing.T, lines ...string) *Info {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return info
}

func TestPendingTools(t *testing.T) {
	toolUse := `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}]}}`
	toolResult := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1"}]}}`

	// A tool_use with no matching tool_result is pending.
	if info := parseLines(t, toolUse); info.PendingTool != "Read" || info.BackgroundActive {
		t.Errorf("unresolved tool_use: PendingTool=%q background=%v, want Read/false", info.PendingTool, info.BackgroundActive)
	}
	// Answered by a tool_result → nothing pending.
	if info := parseLines(t, toolUse, toolResult); info.PendingTool != "" {
		t.Errorf("resolved tool: PendingTool=%q, want empty", info.PendingTool)
	}
	// A backgrounded Bash keeps BackgroundActive, not a foreground pending tool.
	bg := `{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"sleep 999","run_in_background":true}}]}}`
	if info := parseLines(t, bg); !info.BackgroundActive || info.PendingTool != "" {
		t.Errorf("background Bash: background=%v pending=%q, want true/empty", info.BackgroundActive, info.PendingTool)
	}
	// The most recent unresolved foreground tool wins.
	toolUse2 := `{"type":"assistant","message":{"id":"m3","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"make"}}]}}`
	if info := parseLines(t, toolUse, toolUse2); info.PendingTool != "Bash" {
		t.Errorf("most-recent pending = %q, want Bash", info.PendingTool)
	}
}
