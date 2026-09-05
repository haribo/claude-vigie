package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haribo/claude-vigie/internal/api"
)

// #736. `n` promises the session that has been waiting on the operator longest.
// It picked that session from the whole fleet and then placed the cursor in the
// *filtered* list, so a filter that hid the caller left the cursor where it was
// and the detail panel opened on a different session — silently. The operator
// read one session's prompt, branch and last message believing they belonged to
// the one calling.
//
// A filter typed earlier must not veto the jump: it is cleared when, and only
// when, it is what hides the caller.
func jumpFleet() []api.SessionView {
	return []api.SessionView{
		{ID: "alpha", Name: "alpha", Machine: "laptop", Status: "working",
			LastSeenAt: "2026-09-05T10:00:00Z"},
		{ID: "bravo", Name: "bravo", Machine: "server", Status: "waiting", Attention: true,
			LastSeenAt: "2026-09-05T09:00:00Z", StatusChangedAt: "2026-09-05T09:00:00Z"},
	}
}

func jumpModel(filter string) model {
	return model{
		width: 120, height: 40,
		sessions: jumpFleet(),
		sess: sessionsView{
			filter:        filter,
			prevStatus:    map[string]string{},
			prevAttention: map[string]bool{},
			prevCall:      map[string]bool{},
		},
	}
}

func pressN(m model) model {
	return m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
}

// shownInDetail is the session the detail panel renders — the same choice
// viewSessions makes, which is where the wrong session reached the screen.
func shownInDetail(m model) string {
	vis := m.visibleSessions()
	if !m.sess.detail || len(vis) == 0 {
		return ""
	}
	return vis[clamp(m.sess.cursor, len(vis))].ID
}

func TestJumpReachesTheCallerThroughAFilter(t *testing.T) {
	m := pressN(jumpModel("alpha")) // the filter hides bravo, the session calling

	if m.sess.selectedID != "bravo" {
		t.Fatalf("selected %q, want bravo — the jump must pick the session calling", m.sess.selectedID)
	}
	if got := shownInDetail(m); got != "bravo" {
		t.Errorf("the detail panel shows %q, want bravo — the jump opened another session", got)
	}
	if m.sess.filter != "" {
		t.Errorf("filter = %q, want it cleared: it is what hid the caller", m.sess.filter)
	}
	if body := m.viewSessions(); !strings.Contains(body, "bravo") {
		t.Errorf("the rendered view never names bravo:\n%s", body)
	}
}

func TestJumpKeepsAFilterThatHidesNothing(t *testing.T) {
	m := pressN(jumpModel("bravo")) // the caller passes the filter

	if got := shownInDetail(m); got != "bravo" {
		t.Errorf("the detail panel shows %q, want bravo", got)
	}
	if m.sess.filter != "bravo" {
		t.Errorf("filter = %q, want it left alone — it was not in the way", m.sess.filter)
	}
}
