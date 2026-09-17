package transcript

import "testing"

// #834. A session running a persistent `Monitor` read `idle` while the monitor was
// still going, so the board offered it as free and an operator scanning for one
// picked a session that was occupied.
//
// It is #748's defect on a tool that did not exist when that rule was written. The
// launch is answered at once — measured at 1.3–2.7 s across the local corpus, the
// same shape as a backgrounded Bash — and the work carries on afterwards, so the
// pairing resolves and the turn reads finished.
//
// **The criterion is what the input declares, not what the tool is called.**
// `isBackground` keyed on the name `Bash`, and the fix does not swap one name for
// two: a `Monitor` arrives with a `persistent` field, exactly as a Bash arrives with
// `run_in_background`, and either of them says the work outlives the call. Keying on
// a hand-kept list of names is the failure #821 is about.
//
// Line shapes are the observed ones; the commands are placeholders.

const (
	monitorPersistent = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_mon","name":"Monitor","input":{"command":"until curl -sf http://example.test/health; do sleep 5; done","description":"watch the health endpoint","persistent":true,"timeout_ms":600000}}]}}`
	// What Claude Code writes back within seconds, while the monitor runs on.
	monitorStarted = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_mon","content":"Monitor started (task bq7x1a2c3, persistent — runs until TaskStop or session end). You will be notified on each match."}]}}`
	monitorIdle    = `{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"Watching it now."}]}}`

	// The other half of the tool, and the one that must NOT change: a monitor that
	// is not persistent waits for its condition and answers when it is met — 0 to
	// 63 s in the corpus. Its pairing holds on its own while it waits.
	monitorOneShot = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_one","name":"Monitor","input":{"command":"until test -f /tmp/example/done; do sleep 1; done","description":"wait for the marker","persistent":false,"timeout_ms":60000}}]}}`
)

func TestAPersistentMonitorKeepsTheSessionBusy(t *testing.T) {
	info := parseLines(t, monitorPersistent, monitorStarted, monitorIdle)
	if !info.BackgroundActive {
		t.Error("BackgroundActive = false while the monitor is still running — " +
			"the session reads idle and the board offers it as free (#834)")
	}
}

// The regression this fix could plausibly cause, pinned so it cannot happen
// quietly: a one-shot monitor is held by the ordinary pairing while it waits, and
// must not be opened as background work on top of it.
func TestAOneShotMonitorIsNotBackgroundWork(t *testing.T) {
	info := parseLines(t, monitorOneShot)
	if info.BackgroundActive {
		t.Error("a non-persistent monitor was opened as background work; it answers " +
			"when its condition is met, and the pairing already holds the session")
	}
	if info.PendingTool == "" {
		t.Error("PendingTool is empty while the monitor waits — the ordinary pairing " +
			"is what keeps this session working, and it must still apply")
	}
}

// Its own notification still closes it, on either carrier (#748, #818). A
// persistent monitor emits one only when something stops it, which is why the
// session stays `working` until then — that is the state being true, not a latch
// (ADR-0015: a shell that runs for two days is a session working for two days).
func TestAPersistentMonitorIsClosedByItsNotification(t *testing.T) {
	const notify = `{"type":"user","message":{"content":"<task-notification>\n<tool-use-id>toolu_mon</tool-use-id>\n<status>completed</status>\n<summary>monitor stopped</summary>\n</task-notification>"}}`
	info := parseLines(t, monitorPersistent, monitorStarted, monitorIdle, notify)
	if info.BackgroundActive {
		t.Error("the monitor's own terminal notification did not close it")
	}
}
