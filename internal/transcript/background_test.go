package transcript

import "testing"

// #748. A session waiting on a backgrounded command reported `idle`, so the board
// offered it as free and an operator scanning for one interrupted a session that
// was going to resume by itself.
//
// The pairing cannot hold it: Claude Code answers the launch at once — measured
// at 1.8–3.3 s across 1079 launches — so by the time the watcher looks, the
// `tool_use` is resolved and the turn reads finished. What holds it is the
// `<task-notification>`, the same close an async subagent gets.

const (
	bgLaunch = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"gh pr checks 12 --watch","run_in_background":true}}]}}`
	// What Claude Code actually writes back, seconds later, while the command runs.
	bgAccepted = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg","content":"Command running in background with ID: bq7x1a2c3"}]}}`
	bgStop     = `{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"Started it in the background."}]}}`
)

func bgNotification(status string) string {
	return `{"type":"user","message":{"content":"<task-notification>\n<task-id>bq7x1a2c3</task-id>\n<tool-use-id>toolu_bg</tool-use-id>\n<status>` +
		status + `</status>\n<summary>Background command \"watch the checks\" ` + status + `</summary>\n</task-notification>"}}`
}

func TestABackgroundCommandKeepsTheSessionBusyAfterItsLaunchIsAnswered(t *testing.T) {
	info := parseLines(t, bgLaunch, bgAccepted, bgStop)
	if !info.BackgroundActive {
		t.Error("BackgroundActive = false while the command is still running — the session reads idle and the board offers it as free")
	}
	if info.PendingTool != "" {
		t.Errorf("PendingTool = %q, want empty — a background command is not what DETAIL names", info.PendingTool)
	}
}

// Every terminal status closes it. Only `completed` was recognized, and the
// corpus carries 87 `failed` and 13 `killed` against 836 `completed` — one
// command in ten would have stayed open for good.
func TestEveryTerminalNotificationClosesTheCommand(t *testing.T) {
	for _, status := range []string{"completed", "failed", "killed"} {
		info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgNotification(status))
		if info.BackgroundActive {
			t.Errorf("%s: BackgroundActive still true — the session reads busy for the rest of its life", status)
		}
	}
}

// A notification that says the command is still going is not a close.
func TestARunningNotificationLeavesTheCommandOpen(t *testing.T) {
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, bgNotification("running"))
	if !info.BackgroundActive {
		t.Error("a `running` notification closed the command")
	}
}

// The lost-notification case. Nothing closes it — that is the decision, not an
// oversight (ADR-0015).
//
// The prompt used to. #748 kept it as the fallback for a command that never
// reports, and #810 showed the fallback was itself the defect: the operator who
// queues a follow-up is *acting on* the command, and the prompt put the session
// at rest while it ran. A prompt says nothing about a command built to outlive
// the turn.
//
// **A prompt is still not a close, and that is all this test asserts.** Its name
// said `NothingButItsOwnReport` until #842, which was true of the closing rules and
// stopped being so: a stop the session declares also closes a launch, and Claude
// Code sends no notification for a task it was told to stop. The assertion below
// never changed — what changed is that the old name promised an exclusivity the
// rules no longer have.
//
// What is left is a session reading `working` for the rest of its life when a
// notification is genuinely lost. The figure that used to sit here — *about one in
// eight* — has been retired twice since (#818, then #842) and is deliberately not
// replaced: `just residual` measures it, and a number copied into a comment is how
// the last two survived being wrong. Bounded by the session, and in the safe
// direction: wrongly busy costs a missed opportunity, wrongly at rest costs an
// interruption.
func TestAPromptDoesNotCloseABackgroundCommand(t *testing.T) {
	prompt := `{"type":"user","message":{"content":"now do the other thing"}}`
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, prompt)
	if !info.BackgroundActive {
		t.Error("a prompt closed a command that had not reported; the session goes to rest while it is still running")
	}
}

// A notification is not a prompt, however much it looks like one: it must close
// the command it names and leave its siblings alone (#662).
func TestANotificationDoesNotRetireASiblingCommand(t *testing.T) {
	other := `{"type":"assistant","message":{"id":"m3","content":[{"type":"tool_use","id":"toolu_bg2","name":"Bash","input":{"command":"sleep 999","run_in_background":true}}]}}`
	otherAccepted := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg2","content":"Command running in background with ID: bzzz"}]}}`
	info := parseLines(t, bgLaunch, bgAccepted, other, otherAccepted, bgStop, bgNotification("completed"))
	if !info.BackgroundActive {
		t.Error("closing one background command retired the other")
	}
}

// #810. A session babysitting a background command reads `working` (#748), and
// the point of babysitting is queuing the follow-up: the operator types "when the
// CI is done, do X", Claude answers, and the session dropped to `idle` — while
// the command still ran and the session would wake on its own to carry the
// instruction out. On the board, a machine mid-CI looked at rest.
//
// A prompt closes the turn every older tool call belonged to (#483), on the
// grounds that it proves the session moved on. True of a foreground call whose
// result never came; false of a background one, which is built to outlive the
// turn and re-invoke Claude when it finishes. The prompt proves nothing about it.
func TestAPromptDoesNotEndABackgroundCommand(t *testing.T) {
	prompt := `{"type":"user","message":{"content":"when the checks pass, merge it"}}`
	reply := `{"type":"assistant","message":{"id":"m3","content":[{"type":"text","text":"Will do."}]}}`

	info := parseLines(t, bgLaunch, bgAccepted, bgStop, prompt, reply)
	if !info.BackgroundActive {
		t.Error("queuing the follow-up put the session at rest while its command was still running")
	}
}

// The close it keeps is its own: the command reports, and only then.
func TestTheCommandStillClosesOnItsOwnNotification(t *testing.T) {
	prompt := `{"type":"user","message":{"content":"when the checks pass, merge it"}}`
	info := parseLines(t, bgLaunch, bgAccepted, bgStop, prompt, bgNotification("completed"))
	if info.BackgroundActive {
		t.Error("the command reported finished and the session stayed busy")
	}
}

// A foreground tool is untouched: a prompt still ends the turn it belonged to,
// which is what stops a result that never came from pinning the session for the
// rest of its life (#483).
func TestAPromptStillEndsAForegroundTool(t *testing.T) {
	stuck := `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_fg","name":"Read","input":{"file_path":"/x"}}]}}`
	prompt := `{"type":"user","message":{"content":"never mind"}}`

	info := parseLines(t, stuck, prompt)
	if info.PendingTool != "" {
		t.Errorf("PendingTool = %q after a prompt; a tool call whose result never came would pin the session for good", info.PendingTool)
	}
}
