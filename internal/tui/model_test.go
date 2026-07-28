package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestRenderTabBar(t *testing.T) {
	out := renderTabBar(tabMachines)
	for _, want := range []string{"Sessions", "Machines", "Settings"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "Usage") {
		t.Errorf("Usage tab should be gone: %s", out)
	}
}

func TestTabNavigation(t *testing.T) {
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
	shiftTab := tea.KeyMsg{Type: tea.KeyShiftTab}
	var m tea.Model = model{}
	m, _ = m.Update(tabKey)
	if m.(model).tab != tabMachines {
		t.Errorf("Tab from Sessions = %v, want Machines", m.(model).tab)
	}
	m, _ = m.Update(shiftTab)
	if m.(model).tab != tabSessions {
		t.Errorf("Shift+Tab back = %v, want Sessions", m.(model).tab)
	}
	// Shift+Tab from the first tab wraps to the last (Settings).
	m, _ = m.Update(shiftTab)
	if m.(model).tab != tabSettings {
		t.Errorf("Shift+Tab wrap = %v, want Settings", m.(model).tab)
	}
}

func TestViewHasTabBar(t *testing.T) {
	out := model{}.View()
	for _, want := range []string{"Sessions", "Machines", "Settings"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing tab %q", want)
		}
	}
}

func TestCursorAndDetail(t *testing.T) {
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	named := func(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }
	var m tea.Model = model{sessions: []api.SessionView{{ID: "a"}, {ID: "b"}, {ID: "c"}}}

	m, _ = m.Update(key("j")) // down
	m, _ = m.Update(key("j"))
	m, _ = m.Update(key("j")) // clamp at last (index 2)
	if m.(model).cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.(model).cursor)
	}
	m, _ = m.Update(key("k")) // up
	if m.(model).cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.(model).cursor)
	}

	m, _ = m.Update(named(tea.KeyEnter))
	if !m.(model).detail {
		t.Error("enter did not open detail")
	}
	m, _ = m.Update(named(tea.KeyEsc))
	if m.(model).detail {
		t.Error("esc did not close detail")
	}
}

func TestRenderDetailContainsFields(t *testing.T) {
	out := renderDetail(api.SessionView{
		ID: "5c483c16-x", Title: "claude-fleet", Machine: "minet",
		ProjectDir: "/home/haribo/dev/claude-fleet", GitBranch: "develop",
		Model: "claude-opus-4-8", Status: "working", LastTool: "Bash",
		Usage:     api.Usage{InputTokens: 100, OutputTokens: 200},
		StartedAt: "2026-07-26T10:00:00Z", LastSeenAt: "2026-07-26T10:05:00Z",
	})
	for _, want := range []string{"claude-fleet", "5c483c16-x", "minet", "/home/haribo/dev/claude-fleet", "develop", "Bash", "Started", "Output"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("empty history = %q, want empty", got)
	}
	// Braille packs two samples per glyph, so 4 values → 2 glyphs.
	r := []rune(sparkline([]int{0, 1, 2, 4}))
	if len(r) != 2 {
		t.Fatalf("sparkline length = %d, want 2 (braille packs 2/glyph)", len(r))
	}
	for _, g := range r {
		if g < 0x2800 || g > 0x28FF {
			t.Errorf("glyph %q outside the braille range", string(g))
		}
	}
}

func TestRenderSummary(t *testing.T) {
	sessions := []api.SessionView{
		{Status: "working", Usage: api.Usage{OutputTokens: 1000}},
		{Status: "working", Usage: api.Usage{OutputTokens: 500}},
		{Status: "idle", Usage: api.Usage{OutputTokens: 200}},
	}
	out := renderSummary(sessions, []int{1, 2, 2})
	for _, want := range []string{"working 2", "idle 1", "waiting 0", "out ", "activity "} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	if !fuzzyMatch("cf", "claude-fleet") {
		t.Error("cf should match claude-fleet")
	}
	if !fuzzyMatch("FLEET", "claude-fleet") {
		t.Error("case-insensitive match failed")
	}
	if fuzzyMatch("xyz", "claude-fleet") {
		t.Error("xyz should not match")
	}
	if !fuzzyMatch("", "anything") {
		t.Error("empty pattern should always match")
	}
}

func TestVisibleSessionsFilterAndSort(t *testing.T) {
	// showAll isolates sort/filter from the default hide-stale behavior, which
	// is covered separately by TestIsActive.
	m := model{showAll: true, sessions: []api.SessionView{
		{Title: "alpha", Machine: "m1", Status: "idle", Usage: api.Usage{OutputTokens: 100}, LastSeenAt: "2026-07-26T10:00:00Z"},
		{Title: "beta", Machine: "m2", Status: "working", Usage: api.Usage{OutputTokens: 900}, LastSeenAt: "2026-07-26T11:00:00Z"},
		{Title: "gamma", Machine: "m1", Status: "idle", Usage: api.Usage{OutputTokens: 500}, LastSeenAt: "2026-07-26T09:00:00Z"},
	}}

	if vis := m.visibleSessions(); vis[0].Title != "beta" {
		t.Errorf("default (last-seen) sort first = %q, want beta", vis[0].Title)
	}

	m.sortKey = sortTokens
	vis := m.visibleSessions()
	if vis[0].Title != "beta" || vis[2].Title != "alpha" {
		t.Errorf("token sort = %q..%q, want beta..alpha", vis[0].Title, vis[2].Title)
	}

	m.sortKey = sortLastSeen
	m.filter = "m1"
	if vis := m.visibleSessions(); len(vis) != 2 {
		t.Fatalf("filter m1 len = %d, want 2", len(vis))
	}
}

func TestFilterInput(t *testing.T) {
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	named := func(kt tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: kt} }
	var m tea.Model = model{sessions: []api.SessionView{{Title: "claude-fleet"}, {Title: "note"}}}

	m, _ = m.Update(key("/"))
	if !m.(model).filtering {
		t.Fatal("/ did not enter filter mode")
	}
	m, _ = m.Update(key("c"))
	m, _ = m.Update(key("f"))
	if m.(model).filter != "cf" {
		t.Errorf("filter = %q, want cf", m.(model).filter)
	}
	if vis := m.(model).visibleSessions(); len(vis) != 1 || vis[0].Title != "claude-fleet" {
		t.Errorf("filtered = %v", vis)
	}

	m, _ = m.Update(named(tea.KeyEnter))
	if m.(model).filtering {
		t.Error("enter did not leave filter mode")
	}
	m, _ = m.Update(named(tea.KeyEsc))
	if m.(model).filter != "" {
		t.Errorf("esc did not clear filter: %q", m.(model).filter)
	}
}

func TestGroupingClustersByKey(t *testing.T) {
	m := model{
		groupBy: groupMachine,
		showAll: true, // isolate grouping from the default hide-stale behavior
		sessions: []api.SessionView{
			{Title: "a", Machine: "m2", LastSeenAt: "2026-07-26T12:00:00Z"},
			{Title: "b", Machine: "m1", LastSeenAt: "2026-07-26T11:00:00Z"},
			{Title: "c", Machine: "m2", LastSeenAt: "2026-07-26T10:00:00Z"},
		},
	}
	vis := m.visibleSessions()
	// grouped by machine: m1 sessions, then m2 sessions (contiguous)
	if vis[0].Machine != "m1" {
		t.Errorf("first group = %q, want m1", vis[0].Machine)
	}
	if vis[1].Machine != "m2" || vis[2].Machine != "m2" {
		t.Errorf("m2 sessions not contiguous: %v", []string{vis[1].Machine, vis[2].Machine})
	}
}

func TestRenderGroupedTableHasHeaders(t *testing.T) {
	sessions := []api.SessionView{
		{Title: "a", Machine: "m1", Usage: api.Usage{OutputTokens: 100}},
		{Title: "b", Machine: "m1", Usage: api.Usage{OutputTokens: 200}},
		{Title: "c", Machine: "m2", Usage: api.Usage{OutputTokens: 50}},
	}
	out := renderGroupedTable(sessions, 200, -1, groupMachine, sortState{})
	for _, want := range []string{"▸ m1", "▸ m2", "(2 ·"} {
		if !strings.Contains(out, want) {
			t.Errorf("grouped table missing %q:\n%s", want, out)
		}
	}
}

func TestActivitySpark(t *testing.T) {
	if got := activitySpark([]int64{100}); got != "" {
		t.Errorf("single sample = %q, want empty", got)
	}
	// deltas 50, 0, 250 → 3 samples → 2 braille glyphs (2 samples per glyph)
	if got := []rune(activitySpark([]int64{100, 150, 150, 400})); len(got) != 2 {
		t.Errorf("spark length = %d, want 2 (braille packs 2 deltas/glyph)", len(got))
	}
}

func TestFooterHasHints(t *testing.T) {
	out := footer(tabSessions)
	for _, want := range []string{"switch", "select", "filter", "sort", "group", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q: %s", want, out)
		}
	}
}

func TestGroupToggleCycles(t *testing.T) {
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	var m tea.Model = model{}
	m, _ = m.Update(key("g"))
	if m.(model).groupBy != groupMachine {
		t.Errorf("after 1x g: groupBy = %d, want groupMachine", m.(model).groupBy)
	}
	m, _ = m.Update(key("g"))
	m, _ = m.Update(key("g"))
	if m.(model).groupBy != groupNone {
		t.Errorf("after 3x g: groupBy = %d, want groupNone", m.(model).groupBy)
	}
}

func TestStatusRankAndSort(t *testing.T) {
	if statusRank("working") <= statusRank("waiting") || statusRank("idle") <= statusRank("ended") {
		t.Fatal("status rank must be working > waiting > idle > ended")
	}
	s := []api.SessionView{
		{Status: "idle", LastSeenAt: "2026-07-27T10:00:00Z"},
		{Status: "working", LastSeenAt: "2026-07-27T09:00:00Z"},
		{Status: "ended", LastSeenAt: "2026-07-27T11:00:00Z"},
	}
	sortSessions(s, sortStatus, false)
	if s[0].Status != "working" || s[2].Status != "ended" {
		t.Errorf("status sort = %s..%s, want working..ended", s[0].Status, s[2].Status)
	}
	sortSessions(s, sortStatus, true)
	if s[0].Status != "ended" || s[2].Status != "working" {
		t.Errorf("reversed sort = %s..%s, want ended..working", s[0].Status, s[2].Status)
	}
}

func TestRCSortAndFilter(t *testing.T) {
	m := model{sessions: []api.SessionView{
		{Title: "a", Status: "idle", LastSeenAt: "2026-07-27T10:00:00Z"},
		{Title: "b", Status: "idle", RemoteControl: true, LastSeenAt: "2026-07-27T09:00:00Z"},
	}, showAll: true, sortKey: sortRC}
	vis := m.visibleSessions()
	if !vis[0].RemoteControl {
		t.Errorf("rc sort: first = %q, want the rc-active one", vis[0].Title)
	}
	m.filter = "rc"
	vis = m.visibleSessions()
	if len(vis) != 1 || !vis[0].RemoteControl {
		t.Errorf("filter rc = %d rows, want 1 rc-active", len(vis))
	}
}

func TestSettingsEdit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := model{tab: tabSettings, prefs: defaultPrefs()}
	// cursor 0 = hide_ended; space toggles it off
	space := tea.KeyMsg{Type: tea.KeySpace}
	m2, _ := m.Update(space)
	if m2.(model).prefs.hideEnded {
		t.Error("space did not toggle hide_ended off")
	}
	// move to cursor 1 (idle) and cycle it forward
	down := tea.KeyMsg{Type: tea.KeyDown}
	right := tea.KeyMsg{Type: tea.KeyRight}
	m3, _ := m2.Update(down)
	m4, _ := m3.Update(right)
	if m4.(model).prefs.idleHideAfter != 15*time.Minute {
		t.Errorf("idle after cycle = %s, want 15m", m4.(model).prefs.idleHideAfter)
	}
}

func TestCursorTracksSessionOnReorder(t *testing.T) {
	m := model{showAll: true, selectedID: "b", cursor: 1, sessions: []api.SessionView{
		{ID: "a", LastSeenAt: "2026-07-28T10:00:00Z"},
		{ID: "b", LastSeenAt: "2026-07-28T09:00:00Z"},
		{ID: "c", LastSeenAt: "2026-07-28T08:00:00Z"},
	}}
	// last-seen desc → a,b,c; b is at index 1.
	if got := m.cursorForSelection(); got != 1 {
		t.Fatalf("initial cursor = %d, want 1", got)
	}
	// b becomes the most recent → it moves to the top; the cursor must follow.
	m.sessions[1].LastSeenAt = "2026-07-28T11:00:00Z"
	if got := m.cursorForSelection(); got != 0 {
		t.Errorf("after reorder cursor = %d, want 0 (following session b)", got)
	}
	// A pinned session that vanished clamps instead of pointing at the wrong row.
	m.selectedID = "gone"
	if got := m.cursorForSelection(); got < 0 || got > 2 {
		t.Errorf("clamp out of range: %d", got)
	}
}

func TestStaleFetchIgnored(t *testing.T) {
	var m tea.Model = model{
		fetchSeq: 5, appliedSeq: 5,
		sessions: []api.SessionView{{ID: "x", Title: "current"}},
	}
	// A stale fetch (older generation) must NOT overwrite the current state.
	m, _ = m.Update(sessionsMsg{gen: 3, sessions: []api.SessionView{{ID: "x", Title: "stale"}}})
	if m.(model).sessions[0].Title != "current" {
		t.Error("stale fetch overwrote current state")
	}
	// A newer fetch applies.
	m, _ = m.Update(sessionsMsg{gen: 6, sessions: []api.SessionView{{ID: "x", Title: "fresh"}}})
	if m.(model).appliedSeq != 6 || m.(model).sessions[0].Title != "fresh" {
		t.Error("newer fetch not applied")
	}
}
