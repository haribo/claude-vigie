package transcript

import "testing"

// #843. ADR-0015 asks the reader to accept a measured residual, and that figure has
// been wrong three times — each correction found by an unrelated investigation,
// because nothing in the tree measures it. Every number came from a throwaway script
// that reimplemented these rules and no longer exists.
//
// These counts exist so the measurement reads the bookkeeping the status rules
// already keep. A second reader of the transcript is the failure itself: it would
// measure the reader, not vigie.
func TestTheCountsFollowTheClosingRules(t *testing.T) {
	// Launched and left open: no notification, so the session reads `working`.
	info := parseLines(t, bgLaunch, bgAccepted, bgStop)
	if info.BackgroundLaunches != 1 || info.BackgroundOpen != 1 {
		t.Errorf("launched/open = %d/%d, want 1/1", info.BackgroundLaunches, info.BackgroundOpen)
	}

	// Closed by its own terminal notification: still one launch, none open.
	info = parseLines(t, bgLaunch, bgAccepted, bgStop, bgNotification("completed"))
	if info.BackgroundLaunches != 1 || info.BackgroundOpen != 0 {
		t.Errorf("after a close, launched/open = %d/%d, want 1/0", info.BackgroundLaunches, info.BackgroundOpen)
	}

	// A closed launch must stay counted as launched — the residual is a ratio, and
	// forgetting the denominator is how a rate flatters itself.
	if info.BackgroundLaunches == info.BackgroundOpen {
		t.Error("the closed launch vanished from the denominator")
	}
}

// The counts move with the rules rather than beside them: whatever the parser comes
// to treat as background work is what gets counted, with no second list to update.
func TestAPersistentMonitorCountsAsALaunch(t *testing.T) {
	info := parseLines(t, monitorPersistent, monitorStarted, monitorIdle)
	if info.BackgroundLaunches != 1 || info.BackgroundOpen != 1 {
		t.Errorf("launched/open = %d/%d, want 1/1 — a persistent Monitor is background work since #834",
			info.BackgroundLaunches, info.BackgroundOpen)
	}
}
