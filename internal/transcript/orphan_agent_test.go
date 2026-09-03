package transcript

import "testing"

// #662. An async subagent keeps its parent session `working` while it runs
// (#344), and it is closed by a `<task-notification>` naming its launch id. When
// that line never arrives — Claude Code killed mid-flight, or the undocumented
// format drifting — nothing else closed it.
//
// This is #483 one type over. A real prompt has closed orphan *tool* calls since
// then; agents had no equivalent, so the session read `working` at every pause
// between turns while the operator sat at an idle prompt — and vigie is
// observe-only (ADR-0005), so nothing they did could clear it.
//
// The watcher's 30-minute cap bounds it only against silence. A session the
// operator keeps using never goes quiet, so the age never reaches the cap and the
// false `working` outlives the transcript.
const (
	agentLaunch = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_agent1","name":"Task","input":{"description":"catalog the defs"}}]}}`
	agentAck    = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_agent1","content":"Async agent launched successfully"}]}}`
	agentNotif  = `{"type":"user","message":{"content":"<task-notification><tool-use-id>toolu_agent1</tool-use-id><status>completed</status></task-notification>"}}`
)

func TestARealPromptClosesAnOrphanAgent(t *testing.T) {
	// The launch is acknowledged at once, as an async launch always is; the close
	// never comes. Then the operator types, and the session goes on working.
	info := parseLines(t, agentLaunch, agentAck, realPrompt, replyText, laterUse, laterResult)
	if info.AgentsActive != 0 {
		t.Errorf("AgentsActive = %d after a prompt and a completed turn; the session would read working at every pause",
			info.AgentsActive)
	}
}

// The rule must not close an agent that is genuinely still running: only a line
// the *operator* typed counts, and Claude Code's own injected lines are marked
// isMeta — they land in the middle of live work.
func TestAnInjectedLineLeavesARunningAgentAlone(t *testing.T) {
	info := parseLines(t, agentLaunch, agentAck, resumeLine, systemRemind, skillPreamble)
	if info.AgentsActive != 1 {
		t.Errorf("AgentsActive = %d after injected lines only, want 1 — the agent is still running", info.AgentsActive)
	}
}

// And the close that does arrive still works, before any prompt.
func TestTheNotificationStillClosesTheAgent(t *testing.T) {
	info := parseLines(t, agentLaunch, agentAck, agentNotif)
	if info.AgentsActive != 0 {
		t.Errorf("AgentsActive = %d after its completed notification, want 0", info.AgentsActive)
	}
}

// A `<task-notification>` is a user line carrying plain text, exactly like a
// typed prompt. Treating it as one would close *every* in-flight agent instead of
// the one it names — the notification for a finished agent would silently retire
// a sibling that is still running.
func TestANotificationClosesOnlyTheAgentItNames(t *testing.T) {
	const twoAgents = `{"type":"assistant","message":{"id":"m1","content":[` +
		`{"type":"tool_use","id":"toolu_agent1","name":"Task","input":{"description":"first"}},` +
		`{"type":"tool_use","id":"toolu_agent2","name":"Task","input":{"description":"second"}}]}}`

	info := parseLines(t, twoAgents, agentAck, agentNotif)
	if info.AgentsActive != 1 {
		t.Errorf("AgentsActive = %d after a notification naming one of two agents, want 1", info.AgentsActive)
	}
}
