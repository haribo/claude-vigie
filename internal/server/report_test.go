package server

import "testing"

func TestReconcileWatch(t *testing.T) {
	cases := []struct {
		current, source, incoming string
		wantStatus, wantSource    string
	}{
		// A hook-owned active/semantic state survives the watcher's blind idle.
		{"waiting", "hook", "idle", "waiting", "hook"}, // waiting is hook-only
		{"working", "hook", "idle", "working", "hook"}, // #190: an open hook turn is quiet
		{"thinking", "hook", "idle", "thinking", "hook"},
		// #201: the watcher must retract its OWN stale working — no latch.
		{"working", "watch", "idle", "idle", "watch"},
		{"thinking", "watch", "idle", "idle", "watch"},
		// Any positive watcher observation wins and becomes watch-owned.
		{"waiting", "hook", "working", "working", "watch"}, // activity resumes
		{"working", "hook", "error", "error", "watch"},
		{"idle", "watch", "working", "working", "watch"},
		{"working", "hook", "ended", "ended", "watch"}, // process gone
		{"", "", "working", "working", "watch"},        // first observation is the watcher's
	}
	for _, c := range cases {
		gotStatus, gotSource := reconcileWatch(c.current, c.source, c.incoming)
		if gotStatus != c.wantStatus || gotSource != c.wantSource {
			t.Errorf("reconcileWatch(%q,%q,%q) = (%q,%q), want (%q,%q)",
				c.current, c.source, c.incoming, gotStatus, gotSource, c.wantStatus, c.wantSource)
		}
	}
}

func TestDeriveStatusNotification(t *testing.T) {
	cases := []struct{ event, notifType, want string }{
		{"Notification", "permission_prompt", "waiting"}, // the operator is the blocker
		{"Notification", "idle_prompt", "idle"},          // finished; waiting for the next prompt
		{"Notification", "", "waiting"},                  // older Claude Code: keep waiting
		{"Notification", "agent_needs_input", "waiting"},
		{"UserPromptSubmit", "", "working"},
		{"Stop", "", "idle"},
		{"SessionEnd", "", "ended"},
	}
	for _, c := range cases {
		if got := deriveStatus(c.event, c.notifType, ""); got != c.want {
			t.Errorf("deriveStatus(%q, %q) = %q, want %q", c.event, c.notifType, got, c.want)
		}
	}
}
