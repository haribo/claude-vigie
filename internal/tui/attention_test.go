package tui

import (
	"sort"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
)

func TestNextAttention(t *testing.T) {
	sessions := []api.SessionView{
		{ID: "work", Status: "working", StatusChangedAt: "2026-08-02T09:00:00Z"},
		{ID: "new-wait", Status: "waiting", Attention: true, StatusChangedAt: "2026-08-02T10:00:00Z"},
		{ID: "old-err", Status: "error", Attention: true, StatusChangedAt: "2026-08-02T08:00:00Z"},
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
		return model{prefs: prefs{notify: true}, focus: focusOff,
			sess: sessionsView{prevStatus: map[string]string{}, prevAttention: map[string]bool{}}}
	}
	// The daemon marks a blocked session as needing attention (ADR-0011, #617), so
	// the helper does too: a fixture that omits it describes an answer the server
	// never sends.
	sess := func(id, st string) api.SessionView {
		// Name as the daemon derives it — the notification is fed the display name (#618).
		return api.SessionView{ID: id, Title: id, Name: id, Status: st, Attention: status.NeedsAttention(st)}
	}

	// First apply arms the remembered state; a session seen for the first time never
	// notifies, so launching the TUI is silent whatever the fleet is doing.
	m := base().withNotifiedTransitions([]api.SessionView{sess("a", "working"), sess("b", "idle")})
	if len(fired) != 0 {
		t.Fatalf("startup fired %v, want none", fired)
	}
	// Both enter the attention set, so both fire.
	//
	// This assertion used to read "b: idle → waiting does NOT (no working
	// predecessor)". That narrow rule is what #665 removed: it made the terminal
	// the only client that stayed quiet for a permission prompt arriving after a
	// turn had finished, while the GNOME indicator and the README both say any
	// entry into the set calls the operator. Startup silence never depended on it.
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "waiting"), sess("b", "waiting")})
	sort.Strings(fired)
	if len(fired) != 2 || fired[0] != "a:waiting" || fired[1] != "b:waiting" {
		t.Fatalf("fired %v, want both a:waiting and b:waiting", fired)
	}
	// Still waiting on the next tick → no re-notify (edge-triggered).
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "waiting"), sess("b", "waiting")})
	if len(fired) != 2 {
		t.Errorf("re-notified on a held state: %v", fired)
	}
	// Leaving the set re-arms it: the next entry fires again.
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "working"), sess("b", "waiting")})
	m = m.withNotifiedTransitions([]api.SessionView{sess("a", "error"), sess("b", "waiting")})
	if len(fired) != 3 || fired[len(fired)-1] != "a:error" {
		t.Errorf("fired %v, want a:error after a returned to the set", fired)
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
