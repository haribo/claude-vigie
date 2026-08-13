package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestNextAttention(t *testing.T) {
	sessions := []api.SessionView{
		{ID: "work", Status: "working", StatusChangedAt: "2026-08-02T09:00:00Z"},
		{ID: "new-wait", Status: "waiting", StatusChangedAt: "2026-08-02T10:00:00Z"},
		{ID: "old-err", Status: "error", StatusChangedAt: "2026-08-02T08:00:00Z"},
		{ID: "idle", Status: "idle", StatusChangedAt: "2026-08-02T07:00:00Z"},
	}
	if got := nextAttention(sessions); got != "old-err" {
		t.Errorf("nextAttention = %q, want old-err (oldest attention by StatusChangedAt)", got)
	}
	if got := nextAttention([]api.SessionView{{ID: "w", Status: "working"}}); got != "" {
		t.Errorf("nextAttention with no attention = %q, want empty", got)
	}
}

func TestNotifyTransitions(t *testing.T) {
	var fired []string
	orig := notifyFn
	notifyFn = func(name, status string) { fired = append(fired, name+":"+status) }
	t.Cleanup(func() { notifyFn = orig })

	base := func() model {
		return model{prefs: prefs{notify: true}, focus: focusOff, sess: sessionsView{prevStatus: map[string]string{}}}
	}
	sess := func(id, status string) api.SessionView {
		return api.SessionView{ID: id, Title: id, Status: status}
	}

	// First apply arms prevStatus; no working predecessor → nothing fires.
	m := base().withNotifiedTransitions([]api.SessionView{sess("a", "working"), sess("b", "idle")})
	if len(fired) != 0 {
		t.Fatalf("startup fired %v, want none", fired)
	}
	// a: working → waiting fires; b: idle → waiting does NOT (no working predecessor).
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "waiting"), sess("b", "waiting")})
	if len(fired) != 1 || fired[0] != "a:waiting" {
		t.Fatalf("fired %v, want only a:waiting", fired)
	}
	// Still waiting on the next tick → no re-notify (edge-triggered).
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "waiting"), sess("b", "waiting")})
	if len(fired) != 1 {
		t.Errorf("re-notified on a held state: %v", fired)
	}

	// Focus suppresses; a fresh working→error transition must stay silent.
	fired = nil
	mf := base()
	mf.focus = focusOn
	mf = mf.withNotifiedTransitions([]api.SessionView{sess("c", "working")})
	mf = mf.withNotifiedTransitions([]api.SessionView{sess("c", "error")})
	if len(fired) != 0 {
		t.Errorf("focused fired %v, want none", fired)
	}

	// Opt-out suppresses too.
	fired = nil
	mo := base()
	mo.prefs.notify = false
	mo = mo.withNotifiedTransitions([]api.SessionView{sess("d", "working")})
	mo = mo.withNotifiedTransitions([]api.SessionView{sess("d", "waiting")})
	if len(fired) != 0 {
		t.Errorf("opt-out fired %v, want none", fired)
	}
	_ = m
}
