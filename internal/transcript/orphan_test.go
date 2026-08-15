package transcript

import "testing"

// #483. A tool_use whose tool_result never arrives — Claude Code killed while the
// tool was in flight — used to stay pending for the rest of the transcript, so the
// session read `stalled` at every pause between turns, forever, with no way for
// the operator to clear it (ADR-0005).
//
// The lines below are the shapes actually observed in the transcript that exposed
// it (session 884e43f5, a Monitor emitted at 08:09:54Z and never answered): the
// operator's GNOME session died, Claude Code came back an hour later with an
// injected resume line, and the session then ran dozens of complete turns while
// still reporting "stopped at Monitor".
const (
	orphanUse = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Monitor","input":{}}]}}`
	// Claude Code's own resume line: a user line, but machine-injected — isMeta.
	resumeLine = `{"type":"user","isMeta":true,"message":{"content":[{"type":"text","text":"Continue from where you left off."}]}}`
	// What the operator actually typed: a plain string, no tool_result.
	realPrompt    = `{"type":"user","message":{"content":"ma session gnome a été coupée"}}`
	replyText     = `{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"ok"}]}}`
	laterUse      = `{"type":"assistant","message":{"id":"m3","content":[{"type":"tool_use","id":"t2","name":"Read","input":{}}]}}`
	laterResult   = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2"}]}}`
	systemRemind  = `{"type":"user","isMeta":true,"message":{"content":"<system-reminder>\nThe user named this session \"x\".\n</system-reminder>"}}`
	skillPreamble = `{"type":"user","isMeta":true,"message":{"content":[{"type":"text","text":"Base directory for this skill: /tmp/x"}]}}`
)

// A prompt the operator typed ends the turn the orphan belonged to, so the orphan
// stops being reported — including after the session has gone on working.
func TestARealPromptClosesAnOrphanToolCall(t *testing.T) {
	info := parseLines(t, orphanUse, resumeLine, replyText, realPrompt, laterUse, laterResult)
	if info.PendingTool != "" {
		t.Errorf("PendingTool = %q after a prompt and a completed turn; the session would read stalled forever", info.PendingTool)
	}
}

// The same for a backgrounded Bash: an unresolved one keeps the session
// `working`, which latches just as permanently (refineWithTools).
func TestARealPromptClosesAnOrphanBackgroundTask(t *testing.T) {
	bg := `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"sleep 999","run_in_background":true}}]}}`
	if info := parseLines(t, bg, realPrompt); info.BackgroundActive {
		t.Error("BackgroundActive survived a new prompt; the session would read working forever")
	}
}

// The other half of the guard, and the one that decides the rule: Claude Code
// injects its own `user` lines *in the middle of a live tool call*, and closing
// the turn on those would break stalled detection outright. Measured across 313
// local transcripts, keying on isMeta is what makes the difference — without it,
// three of the four turn boundaries found were false.
func TestAnInjectedUserLineDoesNotCloseALiveToolCall(t *testing.T) {
	for _, c := range []struct{ name, line string }{
		{"system reminder", systemRemind},
		{"skill preamble", skillPreamble},
		{"resume line", resumeLine},
	} {
		if info := parseLines(t, orphanUse, c.line); info.PendingTool != "Monitor" {
			t.Errorf("%s: PendingTool = %q, want Monitor — a live tool call was closed by an injected line", c.name, info.PendingTool)
		}
	}
}

// A tool_result is not a prompt: the user line that carries one must keep the rest
// of the turn's tools pending (two tools in flight, one answered).
func TestAToolResultDoesNotCloseTheOtherPendingTools(t *testing.T) {
	two := `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}},{"type":"tool_use","id":"t2","name":"Grep","input":{}}]}}`
	one := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2"}]}}`
	if info := parseLines(t, two, one); info.PendingTool != "Read" {
		t.Errorf("PendingTool = %q, want Read — answering one tool closed the other", info.PendingTool)
	}
}
