package transcript

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// pendingAgents tracks async subagents (the Task/Agent tool) launched from a
// parent transcript but not yet reported finished, so a session whose only work
// runs inside a subagent reads working rather than idle (#344). Unlike a
// foreground tool, an async launch is answered at once by an "Async agent
// launched successfully" tool_result — so it never lingers in pendingTools; the
// real close is a <task-notification> whose <tool-use-id> matches the launch's
// tool_use id. Tracked from the parent transcript alone: no subagent file is
// globbed or parsed on the hot path.
type pendingAgents struct {
	desc  map[string]string // in-flight tool_use id -> agent description
	order []string          // tool_use ids in first-seen order (for the most recent)
}

func newPendingAgents() *pendingAgents {
	return &pendingAgents{desc: map[string]string{}}
}

// addLaunches records the Task/Agent tool_use blocks of an assistant message.
func (p *pendingAgents) addLaunches(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var blocks []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
			Description string `json:"description"`
		} `json:"input"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_use" || b.ID == "" || (b.Name != "Task" && b.Name != "Agent") {
			continue
		}
		if _, ok := p.desc[b.ID]; !ok {
			p.order = append(p.order, b.ID)
		}
		p.desc[b.ID] = b.Input.Description
	}
}

var (
	// A <task-notification> block is injected text in a user line whose content is
	// a JSON string; a completed one closes the agent it names. The format is
	// Claude Code's, undocumented and liable to drift — a missed close is bounded
	// by the watcher's liveness cap (agentWindow), not permanent.
	notifBlockRe = regexp.MustCompile(`(?s)<task-notification>(.*?)</task-notification>`)
	toolUseIDRe  = regexp.MustCompile(`<tool-use-id>\s*(toolu_[A-Za-z0-9]+)\s*</tool-use-id>`)
	completedRe  = regexp.MustCompile(`<status>\s*completed\s*</status>`)
)

// clearNotifications closes the in-flight agents named by a completed
// <task-notification>. The notification rides in a user line whose content is a
// plain string; a non-string content (e.g. a tool_result array) carries none.
func (p *pendingAgents) clearNotifications(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return // content is not a string → no injected notification
	}
	for _, blk := range notifBlockRe.FindAllStringSubmatch(s, -1) {
		body := blk[1]
		if !completedRe.MatchString(body) {
			continue // still running (or resumed) — leave it in flight
		}
		if m := toolUseIDRe.FindStringSubmatch(body); m != nil {
			delete(p.desc, m[1])
		}
	}
}

// resolve returns the number of in-flight agents and a capped "doing" line, e.g.
// "2 agents: Cataloguer defs stdlib" from the most recently launched one still
// running. Empty when none are in flight.
func (p *pendingAgents) resolve() (count int, activity string) {
	count = len(p.desc)
	if count == 0 {
		return 0, ""
	}
	desc := ""
	for i := len(p.order) - 1; i >= 0; i-- {
		if d, ok := p.desc[p.order[i]]; ok {
			desc = d
			break
		}
	}
	noun := "agent"
	if count > 1 {
		noun = "agents"
	}
	activity = fmt.Sprintf("%d %s", count, noun)
	if desc != "" {
		activity += ": " + desc
	}
	return count, capActivity(activity)
}
