package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestIsSingleCell guards the configurable marker (#389): the table pads by rune
// count and vigie carries no display-width dependency, so anything that is not
// exactly one cell would shift every column to its right.
func TestIsSingleCell(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"●", true}, {"◉", true}, {"◌", true}, {"*", true}, {"o", true}, {"é", true},
		{"ｱ", true},  // halfwidth katakana really is one cell
		{"漢", false}, // ideograph: two cells
		{"Ａ", false}, // fullwidth latin: two cells
		{"🔔", false}, // emoji: two cells
		{"", false}, {"ab", false}, {" ", false}, {"\t", false},
		{"́", false},  // combining acute: no cell of its own
		{"●️", false}, // emoji presentation selector makes it two runes and two cells
	} {
		if got := isSingleCell(c.in); got != c.want {
			t.Errorf("isSingleCell(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReplaceLeadingDot(t *testing.T) {
	for _, c := range []struct{ cell, dot, want string }{
		{"● idle", " ", "  idle"},
		{"● error 500", "*", "* error 500"},
		{"◌ stale", " ", "  stale"}, // the dotted ring is a status glyph too (#285)
		{"idle", " ", "idle"},       // no known glyph: untouched
	} {
		if got := replaceLeadingDot(c.cell, c.dot); got != c.want {
			t.Errorf("replaceLeadingDot(%q, %q) = %q, want %q", c.cell, c.dot, got, c.want)
		}
	}
}

func TestCallDot(t *testing.T) {
	on := frame{enabled: true, blinkOn: true, marker: "●"}
	off := frame{enabled: true, blinkOn: false, marker: "●"}
	disabled := frame{enabled: false, blinkOn: false, marker: "●"}

	if on.callDot() != "●" || off.callDot() != " " {
		t.Errorf("blink cycle = %q/%q, want ●/space", on.callDot(), off.callDot())
	}
	if disabled.callDot() != "●" {
		t.Errorf("with blinking off the marker must stay lit, got %q", disabled.callDot())
	}
	if (frame{enabled: true, blinkOn: true}).callDot() != defaultCallMarker {
		t.Error("an unset marker must fall back to the default glyph")
	}
}

// TestBlinkKeepsRowWidth is the regression that matters: both half-cycles must
// render to the same width, or the whole table would jitter once a second.
func TestBlinkKeepsRowWidth(t *testing.T) {
	s := api.SessionView{
		Title: "melonia", Machine: "m", ProjectDir: "/p", Status: "idle",
		CallAt: "2026-08-12T10:00:00Z", CallMessage: "backfill done",
	}
	cols := visibleColumns(columns, 200)
	on := renderRow(cols, s, false, 200, frame{enabled: true, blinkOn: true, marker: "●"})
	off := renderRow(cols, s, false, 200, frame{enabled: true, blinkOn: false, marker: "●"})

	if lipgloss.Width(on) != lipgloss.Width(off) {
		t.Errorf("row width changes with the blink: on=%d off=%d", lipgloss.Width(on), lipgloss.Width(off))
	}
	if !strings.Contains(on, "●") {
		t.Error("the lit half-cycle should show the marker")
	}
	// The status word must stay readable on both frames — that is what makes the
	// dot safe to animate at all (ADR-0010).
	if !strings.Contains(on, "idle") || !strings.Contains(off, "idle") {
		t.Error("the status word must survive both half-cycles")
	}
}

// TestCallTakesTheDoingCell: the call message is the reason the row blinks, so it
// outranks the last tool in DOING.
func TestCallTakesTheDoingCell(t *testing.T) {
	withMsg := api.SessionView{Activity: "Edit render.go", CallAt: "t", CallMessage: "backfill done"}
	if got := activityCell(withMsg); got != "backfill done" {
		t.Errorf("activityCell = %q, want the call message", got)
	}
	noMsg := api.SessionView{Activity: "Edit render.go", CallAt: "t"}
	if got := activityCell(noMsg); got != "called you" {
		t.Errorf("a message-less call should still say so, got %q", got)
	}
	noCall := api.SessionView{Activity: "Edit render.go"}
	if got := activityCell(noCall); got != "Edit render.go" {
		t.Errorf("without a call the activity stays, got %q", got)
	}
}

func TestSummaryShowsCallCountOnlyWhenNonZero(t *testing.T) {
	none := []api.SessionView{{Status: "idle"}}
	counts, _ := summaryParts(none, nil)
	if strings.Contains(strings.Join(counts, " "), "call") {
		t.Error("no call must show no call counter")
	}
	some := []api.SessionView{{Status: "idle", CallAt: "t"}, {Status: "idle"}}
	counts, _ = summaryParts(some, nil)
	joined := strings.Join(counts, " ")
	if !strings.Contains(joined, "call 1") {
		t.Errorf("summary missing the call counter: %q", joined)
	}
	if !strings.HasPrefix(joined, counts[0]) || !strings.Contains(counts[0], "call") {
		t.Errorf("the call counter should lead the summary, got %q", counts[0])
	}
}

// TestNextAttentionPrefersACall: a call is explicit where waiting/error/stalled
// are deductions, so it jumps first — oldest call first within the calls (#389).
func TestNextAttentionPrefersACall(t *testing.T) {
	sessions := []api.SessionView{
		{ID: "waiting-old", Status: "waiting", StatusChangedAt: "2026-08-12T09:00:00Z"},
		{ID: "called-late", Status: "idle", CallAt: "2026-08-12T11:00:00Z"},
		{ID: "called-early", Status: "idle", CallAt: "2026-08-12T10:00:00Z"},
	}
	if got := nextAttention(sessions); got != "called-early" {
		t.Errorf("nextAttention = %q, want called-early", got)
	}
	// With no call, the inferred states keep their existing behavior.
	if got := nextAttention(sessions[:1]); got != "waiting-old" {
		t.Errorf("without a call nextAttention = %q, want waiting-old", got)
	}
}

// TestCallNotifies covers the #260 reuse: a newly raised call notifies, a call
// already there when the TUI launched does not (launching must stay silent).
func TestCallNotifies(t *testing.T) {
	var fired []string
	orig := notifyFn
	notifyFn = func(name, status string) { fired = append(fired, name+":"+status) }
	t.Cleanup(func() { notifyFn = orig })

	m := model{prefs: prefs{notify: true}, focused: false,
		sess: sessionsView{prevStatus: map[string]string{}, prevCall: map[string]bool{}}}

	// First observation: a call already raised must stay silent.
	m = m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "idle", CallAt: "t1"}})
	if len(fired) != 0 {
		t.Errorf("a pre-existing call notified at startup: %v", fired)
	}
	// Same call still up: no repeat.
	m = m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "idle", CallAt: "t1"}})
	if len(fired) != 0 {
		t.Errorf("a standing call notified again: %v", fired)
	}
	// Cleared, then raised again: one notification.
	m = m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "idle"}})
	m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "idle", CallAt: "t2"}})
	if len(fired) != 1 || !strings.Contains(fired[0], "calling you") {
		t.Errorf("a re-raised call should notify once, got %v", fired)
	}
}

// TestBlinkTickRunsOnlyWhileCalling: the animation must not outlive its reason,
// and must never raise the ambient 5 s poll into a permanent 500 ms one.
func TestBlinkTickRunsOnlyWhileCalling(t *testing.T) {
	m := model{prefs: prefs{blink: true, callMarker: "●"}}
	if _, cmd := m.withBlinkTick(); cmd != nil {
		t.Error("no call: no tick should be scheduled")
	}

	m.sessions = []api.SessionView{{ID: "a", Status: "idle", CallAt: "t"}}
	got, cmd := m.withBlinkTick()
	if cmd == nil {
		t.Fatal("a call should start the blink tick")
	}
	m = got.(model)
	if !m.sess.blinkTicking {
		t.Error("the tick must be marked in flight so a second one never stacks")
	}
	if _, cmd := m.withBlinkTick(); cmd != nil {
		t.Error("a tick is already in flight; a second must not be scheduled")
	}

	// The call clears: the next tick stops the animation and leaves the dot lit.
	m.sessions = nil
	next, cmd := m.Update(blinkMsg{})
	m = next.(model)
	if cmd != nil {
		t.Error("with nothing calling, the tick must not reschedule")
	}
	if m.sess.blinkTicking || m.sess.blinkOn {
		t.Errorf("animation state not reset: ticking=%v on=%v", m.sess.blinkTicking, m.sess.blinkOn)
	}

	// Blinking disabled by preference: never any tick.
	off := model{prefs: prefs{blink: false}, sessions: []api.SessionView{{ID: "a", CallAt: "t"}}}
	if _, cmd := off.withBlinkTick(); cmd != nil {
		t.Error("blink = false must schedule no tick at all")
	}
}
