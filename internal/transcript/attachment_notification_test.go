package transcript

import "testing"

// #818. A `<task-notification>` is delivered two ways, and vigie read one of them.
//
// When a background command ends while the session is at rest, the notification
// arrives on a `user` line and closes the command. When it ends **while a turn is
// still running** — which is the normal case for something launched to outlive the
// turn — Claude Code queues it instead: the same text arrives on an `attachment`
// line as a `queued_command` with `commandMode: "task-notification"`. The parser's
// switch handles `assistant`, `user` and `system`, so that line reached `applyMeta`
// and nothing else, and the command stayed open for the rest of the session.
//
// Nothing could clear it: since #812 only its notification closes a background
// command, and the operator's next prompt deliberately does not (#810). Measured
// over the local corpus, 114 of 789 launches (14 %) have their terminal
// notification *only* on this carrier, and one session in five that vigie shows
// ends carrying a status that will never correct itself.
//
// The shape below is the observed one, tags and all — including `<output-file>`,
// which the `user`-line fixtures do not carry. Only the structure is copied; no
// recorded content is reproduced here (ADR-0016).

func bgAttachmentNotification(status string) string {
	return `{"type":"attachment","attachment":{"type":"queued_command",` +
		`"commandMode":"task-notification","prompt":"<task-notification>\n` +
		`<task-id>bq7x1a2c3</task-id>\n<tool-use-id>toolu_bg</tool-use-id>\n` +
		`<output-file>/tmp/example/bq7x1a2c3.output</output-file>\n<status>` + status +
		`</status>\n<summary>Background command finished</summary>\n</task-notification>"}}`
}

func TestAnAttachmentDeliveredNotificationClosesTheCommand(t *testing.T) {
	for _, status := range []string{"completed", "failed", "killed"} {
		info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgAttachmentNotification(status))
		if info.BackgroundActive {
			t.Errorf("%s: the command is still open — the session reads `working` for the rest of its life, "+
				"and no prompt can clear it (#810, #812)", status)
		}
	}
}

// The guard the other carrier already has: a notification that says the command is
// still going is not a close, whichever line it arrives on.
func TestARunningAttachmentNotificationLeavesTheCommandOpen(t *testing.T) {
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgAttachmentNotification("running"))
	if !info.BackgroundActive {
		t.Error("a `running` notification on an attachment closed the command")
	}
}

// **An attachment is not a prompt, and must never close the turn.** A prompt
// retires every older unresolved `tool_use` (#483) on the grounds that the session
// moved on. An attachment proves nothing of the sort — it is Claude Code queueing
// its own message mid-turn — and treating it as one would retire a foreground tool
// call that is genuinely still running, which is the #810 defect one carrier along.
func TestAnAttachmentNeverClosesTheTurn(t *testing.T) {
	const foreground = `{"type":"assistant","message":{"id":"m9","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_fg","name":"Bash","input":{"command":"just code-check"}}]}}`
	info := parseLines(t, foreground, bgAttachmentNotification("completed"))
	if info.PendingTool == "" {
		t.Error("the attachment retired a foreground tool call that is still running — " +
			"it was read as a prompt, which is exactly what it is not")
	}
}

// Subagents ride the same carrier, and the same fix has to reach them. Bounded
// rather than permanent — a prompt still closes an agent (`agents.go` closeTurn) —
// so this is a shorter latch, not a lifelong one.
func TestAnAttachmentDeliveredNotificationClosesASubagent(t *testing.T) {
	const launch = `{"type":"assistant","message":{"id":"m1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_A","name":"Task","input":{"description":"Find X","subagent_type":"Explore"}}]}}`
	const answered = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_A","content":[{"type":"text","text":"Async agent launched successfully. agentId: a1"}]}]}}`
	const notify = `{"type":"attachment","attachment":{"type":"queued_command","commandMode":"task-notification","prompt":"<task-notification>\n<tool-use-id>toolu_A</tool-use-id>\n<status>completed</status>\n<summary>done</summary>\n</task-notification>"}}`

	if info := parseLines(t, launch, answered); info.AgentsActive != 1 {
		t.Fatalf("setup: AgentsActive = %d, want 1", info.AgentsActive)
	}
	if info := parseLines(t, launch, answered, notify); info.AgentsActive != 0 {
		t.Errorf("AgentsActive = %d, want 0 — the agent's own close was delivered and ignored", info.AgentsActive)
	}
}
