package transcript

import "encoding/json"

// pendingTools tracks tool_use blocks awaiting a tool_result across a transcript,
// so the watcher can tell a genuinely-hung tool (a stalled turn) from a finished
// one (#256). The pairing is tool_use.id ↔ tool_result.tool_use_id.
type pendingTools struct {
	meta  map[string]toolMeta
	order []string // tool_use ids in first-seen order, for "most recent pending"
}

type toolMeta struct {
	name       string
	background bool
}

func newPendingTools() *pendingTools {
	return &pendingTools{meta: map[string]toolMeta{}}
}

// addToolUses records the tool_use blocks of an assistant message. A Bash with
// run_in_background is a background task — it legitimately runs long.
func (p *pendingTools) addToolUses(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var blocks []struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_use" || b.ID == "" {
			continue
		}
		if _, ok := p.meta[b.ID]; !ok {
			p.order = append(p.order, b.ID)
		}
		p.meta[b.ID] = toolMeta{name: b.Name, background: isBackground(b.Name, b.Input)}
	}
}

// clearToolResults drops the tool_use ids answered by a tool_result block (Claude
// Code writes them in user messages) and reports whether it saw any. Non-array
// content (a plain user string) is ignored, so a normal message never disturbs
// the pairing — and answers false, which is what marks it a prompt (see
// closeTurn).
func (p *pendingTools) clearToolResults(raw json.RawMessage) (answered bool) {
	if len(raw) == 0 {
		return false
	}
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			delete(p.meta, b.ToolUseID)
			answered = true
		}
	}
	return answered
}

// closeTurn drops every still-unresolved tool_use, called when a prompt the
// operator typed opens a new turn (#483).
//
// A tool_result that never arrives — Claude Code killed while the tool was in
// flight — otherwise leaves its tool_use in the map for the rest of the
// transcript, and the session reads `stalled` at every pause between turns for
// the rest of its life. Nothing can clear it: vigie is observe-only towards the
// session (ADR-0005), so a permanent false "the operator is needed here" is worse
// than a missed one. A prompt proves the session moved on, so a call from before
// it cannot be what the current turn is parked on.
//
// The direction of the residual error is the safe one. If a queued prompt were
// ever written before the result of a tool still genuinely running, this would
// miss a stall rather than invent one — and across 313 local transcripts it never
// happened: the rule fired exactly once, on the one dead tool call.
func (p *pendingTools) closeTurn() {
	if len(p.meta) == 0 {
		return
	}
	p.meta = map[string]toolMeta{}
	p.order = p.order[:0]
}

// resolve returns the most recent unresolved foreground tool's name (for the
// stalled/executing message) and whether any unresolved tool is a background task
// (which keeps the session working).
func (p *pendingTools) resolve() (pendingTool string, backgroundActive bool) {
	for _, m := range p.meta {
		if m.background {
			backgroundActive = true
			break
		}
	}
	for i := len(p.order) - 1; i >= 0; i-- {
		if m, ok := p.meta[p.order[i]]; ok && !m.background {
			return m.name, backgroundActive
		}
	}
	return "", backgroundActive
}

// isBackground reports whether a tool call is a backgrounded Bash.
func isBackground(name string, input json.RawMessage) bool {
	if name != "Bash" {
		return false
	}
	var in struct {
		RunInBackground bool `json:"run_in_background"`
	}
	_ = json.Unmarshal(input, &in)
	return in.RunInBackground
}
