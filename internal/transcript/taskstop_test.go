package transcript

import "testing"

// #842. A session that stops its own background command keeps reading `working`
// until it ends. The operator scanning for a free session skips one that is free,
// and no prompt brings it back (#810, #812).
//
// The session said so. Claude Code answers `TaskStop` with
// `Successfully stopped task: <id>`, and emits **no** `<task-notification>` for a
// task it was told to stop — so the only close vigie reads never comes, while the
// transcript carries the stop in plain sight.
//
// **A declared stop is an observation, not a clock**, so
// [ADR-0015](../../docs/adr/0015-no-timer-decides-what-vigie-cannot-observe.md)
// is untouched: nothing here concludes from elapsed time.
//
// The pairing runs through the task id, which only the launch's own result names.
// Keyed on the input field rather than the tool's name — the principle
// `isBackgroundWork` was rebuilt on in #834, and the reason #821 exists.
//
// Line shapes are the observed ones; the commands are placeholders.

const (
	stopLaunch  = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"just code-check","run_in_background":true}}]}}`
	stopStarted = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_bg","content":"Command running in background with ID: bq7x1a2c3. Output is being written to /tmp/example/bq7x1a2c3.output"}]}}`
	stopCall    = `{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"toolu_stop","name":"TaskStop","input":{"task_id":"bq7x1a2c3"}}]}}`
	stopDone    = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stop","content":"{\"message\":\"Successfully stopped task: bq7x1a2c3 (just code-check)\"}"}]}}`
)

func TestAStopTheSessionDeclaredClosesTheLaunch(t *testing.T) {
	if info := parseLines(t, stopLaunch, stopStarted); !info.BackgroundActive {
		t.Fatal("setup: the launch should be open while it runs")
	}
	info := parseLines(t, stopLaunch, stopStarted, stopCall, stopDone)
	if info.BackgroundActive {
		t.Error("the session stopped its own command and still reads `working` — " +
			"no notification is emitted for a stopped task, so nothing else will ever close it (#842)")
	}
	if info.BackgroundLaunches != 1 {
		t.Errorf("launches = %d, want 1 — a closed launch stays in the denominator", info.BackgroundLaunches)
	}
}

// A persistent `Monitor` carries its task id in a different sentence, and is the
// case that brought this to light.
func TestAStoppedMonitorClosesToo(t *testing.T) {
	const launch = `{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"toolu_mon","name":"Monitor","input":{"command":"until curl -sf http://example.test/health; do sleep 5; done","persistent":true,"timeout_ms":600000}}]}}`
	const started = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_mon","content":"Monitor started (task bmon12345, persistent — runs until TaskStop or session end). You will be notified on each match."}]}}`
	const call = `{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"toolu_stop","name":"TaskStop","input":{"task_id":"bmon12345"}}]}}`
	const done = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stop","content":"{\"message\":\"Successfully stopped task: bmon12345 (watch the health endpoint)\"}"}]}}`

	if info := parseLines(t, launch, started, call, done); info.BackgroundActive {
		t.Error("a stopped persistent Monitor still reads as running")
	}
}

// **A stop that did not succeed closes nothing.** The work is still going, and
// reading the request rather than its outcome would put the session at rest while
// its command runs — the defect #810 was filed for, one signal along.
func TestAStopThatDidNotSucceedLeavesTheLaunchOpen(t *testing.T) {
	const failed = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stop","is_error":true,"content":"{\"message\":\"No such task: bq7x1a2c3\"}"}]}}`
	if info := parseLines(t, stopLaunch, stopStarted, stopCall, failed); !info.BackgroundActive {
		t.Error("the stop failed and the command is still running, yet the session was put at rest")
	}
}

// A stop naming a task this transcript never launched closes nothing — and must not
// close something else by accident.
func TestAStopForAnUnknownTaskClosesNothing(t *testing.T) {
	const other = `{"type":"assistant","message":{"id":"m2","content":[{"type":"tool_use","id":"toolu_stop","name":"TaskStop","input":{"task_id":"bsomeoneelse"}}]}}`
	const done = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stop","content":"{\"message\":\"Successfully stopped task: bsomeoneelse (nothing here)\"}"}]}}`
	if info := parseLines(t, stopLaunch, stopStarted, other, done); !info.BackgroundActive {
		t.Error("a stop for another task closed this session's launch")
	}
}
