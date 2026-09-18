package transcript

import (
	"encoding/json"
	"regexp"
)

// pendingTools tracks tool_use blocks awaiting a tool_result across a transcript,
// so the watcher can tell a session waiting on a command from a finished
// one (#256). The pairing is tool_use.id ↔ tool_result.tool_use_id.
type pendingTools struct {
	meta  map[string]toolMeta
	order []string // tool_use ids in first-seen order, for "most recent pending"
	// bgLaunched counts every background launch seen, closed ones included, because
	// `meta` only holds what is still open. It exists so the residual can be
	// measured against what was launched rather than re-derived by a second reader
	// of the transcript: a figure ADR-0015 rests on has been wrong three times, and
	// each time it came from a throwaway script reimplementing these rules (#843).
	bgLaunched int
	// byTask maps a background task's own id to the tool_use that started it. Only
	// the launch's result names that id — `Command running in background with ID: …`
	// for a Bash, `Monitor started (task …` for a Monitor — so the pairing has to be
	// built as the transcript is read, and it is what lets a later stop name which
	// launch it ended (#842).
	byTask map[string]string
	// pendingStops holds TaskStop calls whose outcome has not landed yet, keyed by
	// the call's own tool_use id. A stop is read on its *result*, never on the
	// request: a stop that failed leaves the work running, and closing on the ask
	// would put the session at rest while its command runs — #810's defect, one
	// signal along.
	pendingStops map[string]string
}

type toolMeta struct {
	name       string
	background bool
}

func newPendingTools() *pendingTools {
	return &pendingTools{
		meta:   map[string]toolMeta{},
		byTask: map[string]string{}, pendingStops: map[string]string{},
	}
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
		if id := stopsTask(b.Input); id != "" {
			// The call declares which task it stops; the outcome decides whether it
			// counts. Keyed on the field, not on the tool's name (#821, #834).
			p.pendingStops[b.ID] = id
		}
		bg := isBackgroundWork(b.Input)
		if bg {
			p.bgLaunched++
		}
		p.meta[b.ID] = toolMeta{name: b.Name, background: bg}
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
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type != "tool_result" || b.ToolUseID == "" {
			continue
		}
		answered = true
		// Two things are read out of the result's own text, and only here: the task
		// id a launch was given, and the outcome of a stop. Neither is in any input,
		// so the pairing between a stop and the launch it ends exists nowhere else.
		txt := resultText(b.Content)
		p.noteLaunchID(b.ToolUseID, txt)
		p.closeStopped(b.ToolUseID, txt)
		// A backgrounded Bash is answered while it is still running. Claude Code
		// writes `Command running in background with ID: …` back within seconds —
		// 1.8 to 3.3 across 1079 launches in the local corpus — so this result says
		// the command *started*, not that it finished. Deleting on it resolved the
		// pairing mid-command, the turn read finished, and a session waiting on a CI
		// watch or a long build reported `idle` (#748).
		//
		// Two things close it, and a prompt is not one of them (#810, closeTurn): its
		// own terminal <task-notification>, like an async subagent, and a stop the
		// session declared, which Claude Code answers but never notifies (#842).
		// Neither is a timer — see docs/design/session-status.md § 2.
		if m, ok := p.meta[b.ToolUseID]; ok && m.background {
			continue
		}
		delete(p.meta, b.ToolUseID)
	}
	return answered
}

// clearBackgroundNotifications closes the background commands named by a terminal
// <task-notification>. Claude Code emits the same notification for a backgrounded
// Bash as for an async subagent, keyed on the same <tool-use-id>, so this is the
// subagent close (#344) applied to the second type that needs it (#748).
//
// The notification rides in a user line whose content is a plain string; a
// non-string content (a tool_result array) carries none.
func (p *pendingTools) clearBackgroundNotifications(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	p.clearBackgroundNotificationsIn(s)
}

// clearBackgroundNotificationsIn is the scan itself, over text from whichever line
// carried it — see clearNotificationsIn for why there are two carriers (#818).
func (p *pendingTools) clearBackgroundNotificationsIn(s string) {
	for _, blk := range notifBlockRe.FindAllStringSubmatch(s, -1) {
		body := blk[1]
		if !terminalRe.MatchString(body) {
			continue // still running — leave it in flight
		}
		if m := toolUseIDRe.FindStringSubmatch(body); m != nil {
			if meta, ok := p.meta[m[1]]; ok && meta.background {
				delete(p.meta, m[1])
			}
		}
	}
}

// closeTurn drops every still-unresolved tool_use, called when a prompt the
// operator typed opens a new turn (#483).
//
// A tool_result that never arrives — Claude Code killed while the tool was in
// flight — otherwise leaves its tool_use in the map for the rest of the
// transcript, and the session reads as still working at every pause between turns for
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
	// A backgrounded command survives the turn it was launched in, so a prompt
	// says nothing about it. The rule above is that a prompt proves the session
	// moved on — true of a foreground call whose result never came, and false of a
	// command built to outlive the turn and re-invoke Claude when it finishes. The
	// operator who queues the follow-up ("when the CI is done, do X") is the
	// clearest case: the prompt is *because* of the command, and it put the
	// session at rest while it was still running (#810).
	//
	// What closes one is its own `<task-notification>`, and nothing else. That is
	// deliberate and it has a price, stated in session-status.md § 2 and in
	// ADR-0015: a notification that never comes leaves the session reading
	// `working` for the rest of its life. Bounded by the session, not beyond it —
	// a process found gone is `ended` before the tool refinements are consulted —
	// and in the safe direction, since a session wrongly shown busy costs a missed
	// opportunity where one wrongly shown at rest costs an interruption.
	kept := make(map[string]toolMeta, len(p.meta))
	order := p.order[:0:0]
	for _, id := range p.order {
		if m, ok := p.meta[id]; ok && m.background {
			kept[id] = m
			order = append(order, id)
		}
	}
	p.meta, p.order = kept, order
}

// resolve returns the most recent unresolved foreground tool's name (for the
// DETAIL message) and whether any unresolved tool is a background task
// (which keeps the session working).
// backgroundCounts returns how many background launches this transcript carried and
// how many are still open at this point. Both are read from the same bookkeeping the
// status rules use, so a measurement cannot drift from the behavior it describes.
func (p *pendingTools) backgroundCounts() (launched, open int) {
	for _, m := range p.meta {
		if m.background {
			open++
		}
	}
	return p.bgLaunched, open
}

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

var (
	// taskIDInResult reads the task id out of a launch's own answer. Two sentences
	// carry it, one per tool, and only there: the launch's input never names it.
	taskIDInResult = regexp.MustCompile(`(?:Command running in background with ID:\s*|Monitor started \(task\s*)([A-Za-z0-9]+)`)
	// stoppedTask matches the answer Claude Code gives a stop that worked. The
	// wording is the contract: a failed stop says something else, and must not close
	// anything.
	stoppedTask = regexp.MustCompile(`Successfully stopped task:\s*([A-Za-z0-9]+)`)
)

// resultText flattens a tool_result's content to the text it carries. Claude Code
// writes it either as a plain string or as a list of blocks, and the two sentences
// this file reads appear in both shapes.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var out string
	for _, b := range blocks {
		out += b.Text + " "
	}
	return out
}

// stopsTask returns the task id a call declares it will stop, or "".
func stopsTask(input json.RawMessage) string {
	var in struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(input, &in)
	return in.TaskID
}

// noteLaunchID records the task id a background launch was given, so a later stop
// can name it. Ignored for anything that is not an open background launch: a task
// id from a Monitor that is not persistent belongs to work vigie never opened, and
// stopping it must close nothing (#834, #842).
func (p *pendingTools) noteLaunchID(toolUseID, text string) {
	m, ok := p.meta[toolUseID]
	if !ok || !m.background {
		return
	}
	if id := taskIDInResult.FindStringSubmatch(text); id != nil {
		p.byTask[id[1]] = toolUseID
	}
}

// closeStopped closes the launch a successful stop names. Called with a stop call's
// own result, so the outcome is what decides — see pendingStops.
func (p *pendingTools) closeStopped(stopToolUseID, text string) {
	declared, ok := p.pendingStops[stopToolUseID]
	if !ok {
		return
	}
	delete(p.pendingStops, stopToolUseID)
	m := stoppedTask.FindStringSubmatch(text)
	if m == nil || m[1] != declared {
		return // the stop did not succeed, or reports another task
	}
	if launch, ok := p.byTask[declared]; ok {
		delete(p.meta, launch)
	}
}

// isBackgroundWork reports whether a tool call starts work that outlives it — the
// call is answered at once and the work carries on, so the tool_use/tool_result
// pairing resolves while the session is still occupied (#748).
//
// **It reads what the input declares, never what the tool is called.** It used to
// require the name `Bash`, and a persistent `Monitor` — answered just as fast, and
// running until something stops it — opened nothing, so a session watching one read
// `idle` and the board offered it as free (#834). Swapping one name for a list of
// two would be the hand-kept list #821 is about: a tool added later would fall
// straight through it, which is how `stalled` slipped past the allow list in #256.
//
// A field is the right key because it is the tool *stating* the property. The
// counterpart matters as much: a `Monitor` that is **not** persistent answers when
// its condition is met — 0 to 63 s measured — so the ordinary pairing already holds
// the session, and opening it here on top would be a second claim on the same work.
// isBackground reports whether a tool call is a backgrounded Bash.
func isBackgroundWork(input json.RawMessage) bool {
	var in struct {
		// A Bash launched with run_in_background. Answered in 1.8–3.3 s across 1079
		// launches while the command runs on (#748).
		RunInBackground bool `json:"run_in_background"`
		// A Monitor declared persistent: it runs "until TaskStop or session end" and
		// is answered in 1.3–2.7 s, the same shape one tool along (#834).
		Persistent bool `json:"persistent"`
	}
	_ = json.Unmarshal(input, &in)
	return in.RunInBackground || in.Persistent
}
