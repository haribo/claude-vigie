package transcript

import (
	"path/filepath"
	"testing"
)

// TestInFlightAgents is the #344 regression: an async Task/Agent launch is
// answered immediately by an "Async agent launched" tool_result, so it never
// stays a pending foreground tool; the session must still count the agent as
// in-flight until its <task-notification> arrives — otherwise a session whose
// only work runs in a subagent reads idle. Line shapes are the observed ones
// (claude 2.1.x).
func TestInFlightAgents(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jsonl")

	launch := []string{
		`{"type":"assistant","message":{"id":"m1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_A","name":"Task","input":{"description":"Find X","subagent_type":"Explore"}},{"type":"tool_use","id":"toolu_B","name":"Task","input":{"description":"Find Y","subagent_type":"Explore"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_A","content":[{"type":"text","text":"Async agent launched successfully. agentId: a1"}]},{"type":"tool_result","tool_use_id":"toolu_B","content":[{"type":"text","text":"Async agent launched successfully. agentId: a2"}]}]}}`,
	}

	writeJSONL(t, p, launch...)
	info, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.AgentsActive != 2 {
		t.Fatalf("AgentsActive = %d, want 2 (both launches still running)", info.AgentsActive)
	}
	if info.PendingTool != "" {
		t.Errorf("a launched Task must not read as a pending foreground tool: %q", info.PendingTool)
	}
	if info.AgentActivity != "2 agents: Find Y" {
		t.Errorf("AgentActivity = %q, want %q", info.AgentActivity, "2 agents: Find Y")
	}

	// One finishes: its <task-notification> (injected text, content is a string)
	// closes the matching <tool-use-id>.
	done := append(append([]string{}, launch...),
		`{"type":"user","message":{"content":"<task-notification>\n<task-id>t1</task-id>\n<tool-use-id>toolu_A</tool-use-id>\n<status>completed</status>\n<summary>Agent \"Find X\" finished</summary>\n</task-notification>"}}`,
	)
	writeJSONL(t, p, done...)
	info, err = Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.AgentsActive != 1 {
		t.Fatalf("AgentsActive = %d, want 1 (one still running)", info.AgentsActive)
	}
	if info.AgentActivity != "1 agent: Find Y" {
		t.Errorf("AgentActivity = %q, want %q", info.AgentActivity, "1 agent: Find Y")
	}
}
