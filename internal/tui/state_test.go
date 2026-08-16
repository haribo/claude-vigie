package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-vigie/internal/api"
)

// #494. Six health indicators sat scattered across the Sessions tab, each with
// its own glyph, wording and location. Five asked one question at five
// granularities — *is what I am looking at true?* — and the sixth was not about
// vigie at all. They become one pill and one modal
// (docs/design/sessions-chrome.md § 3–5).

func healthyModel() model {
	m := stubModel()
	m.width, m.height = 200, 40
	m.clock = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	m.sseLive = true
	m.gotWatcher = true
	m.watcherSeen = time.Now().UTC().Format(time.RFC3339)
	m.platform = api.PlatformStatus{Indicator: "none", Description: "All Systems Operational"}
	m.daemonVersion = api.VersionInfo{Version: "dev", Commit: "none"}
	m.usage = api.UsageReport{FetchedAt: "2026-08-16T11:58:00Z"}
	return m
}

// rowsByLabel indexes the modal's rows so a test can name the layer it means.
func rowsByLabel(m model) map[string]stateRow {
	out := map[string]stateRow{}
	for _, r := range m.stateRows() {
		out[r.label] = r
	}
	return out
}

func TestAHealthyChainIsGreen(t *testing.T) {
	m := healthyModel()
	if got := m.stateLevel(); got != levelOK {
		t.Errorf("level = %v, want green; rows: %+v", got, m.stateRows())
	}
	for _, r := range m.stateRows() {
		if r.level != levelOK {
			t.Errorf("%q is %v on a healthy fleet: %s", r.label, r.level, r.detail)
		}
	}
}

// The sorting criterion is not severity: it is whether something on screen is
// false. A Claude outage is severe, and vigie is reporting it perfectly well.
func TestAClaudeOutageIsAmberAndGreysNothing(t *testing.T) {
	m := healthyModel()
	m.platform = api.PlatformStatus{Indicator: "major", Description: "Elevated error rates"}

	if got := m.stateLevel(); got != levelDegrade {
		t.Errorf("a Claude outage made the pill %v, want amber — vigie is correctly reporting sessions that are correctly erroring", got)
	}
	for _, r := range m.stateRows() {
		if r.level == levelUnknown {
			t.Errorf("%q is unknown during a Claude outage — the link is intact, so every layer is observed", r.label)
		}
	}
}

// When the server is unreachable the TUI loses the ability to observe, not merely
// the data. Showing the last known watcher or platform value as if it were
// current is the lie #449 and #456 exist to prevent.
func TestAnUnreachableServerSplitsObservedFromUnknown(t *testing.T) {
	m := healthyModel()
	m.err = errFake{}
	m.refreshFailed = map[string]bool{srcSessions: true, srcUsage: true}

	if got := m.stateLevel(); got != levelBroken {
		t.Errorf("level = %v, want red — the screen may be lying", got)
	}
	rows := rowsByLabel(m)
	for _, label := range []string{"vigie server", "sessions"} {
		if rows[label].level != levelBroken {
			t.Errorf("%q is %v, want red: the TUI established this failure itself", label, rows[label].level)
		}
	}
	for _, label := range []string{"watcher", "claude platform", "client / daemon"} {
		if rows[label].level != levelUnknown {
			t.Errorf("%q is %v, want grey: with no channel there is no information, good or bad", label, rows[label].level)
		}
	}
	// The snapshot's age is known offline; greying it would hide the one thing
	// still true about it.
	usage := rows["usage snapshot"]
	if usage.level != levelDegrade {
		t.Errorf("usage snapshot is %v, want amber", usage.level)
	}
	if !strings.Contains(usage.detail, "old") || !strings.Contains(usage.detail, "cannot refresh") {
		t.Errorf("the usage row lost its age or its reason: %q", usage.detail)
	}
}

// An unknown is the absence of a level, not a level: it must never color the
// pill by itself.
func TestUnknownAloneDoesNotColorThePill(t *testing.T) {
	m := healthyModel()
	m.gotWatcher = false                       // unknown
	m.platform = api.PlatformStatus{}          // unknown
	m.daemonVersion = api.VersionInfo{}        // unknown
	m.usage = api.UsageReport{}                // unknown
	if got := m.stateLevel(); got != levelOK { // the link and the sessions are fine
		t.Errorf("level = %v, want green — nothing observed is wrong, three things are simply unknown", got)
	}
}

// A watcher that stopped reporting freezes every status on screen, which is the
// definition of red here.
func TestAMissingWatcherIsRed(t *testing.T) {
	m := healthyModel()
	m.watcherSeen = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if got := m.stateLevel(); got != levelBroken {
		t.Errorf("level = %v, want red: no watcher means the statuses may be frozen", got)
	}
}

// The version drift was buried in Settings although it fails confusingly on a
// fleet (#341). Nothing on screen is false, so it is amber.
func TestAVersionDriftIsAmberAndVisible(t *testing.T) {
	m := healthyModel()
	m.daemonVersion = api.VersionInfo{Version: "9.9.9", Commit: "abcdef"}
	if got := m.stateLevel(); got != levelDegrade {
		t.Errorf("level = %v, want amber", got)
	}
	if d := rowsByLabel(m)["client / daemon"].detail; !strings.Contains(d, "9.9.9") {
		t.Errorf("the drift row does not name the daemon build: %q", d)
	}
}

// The whole design rests on the corner never growing: no text ever appears beside
// the pill, so the table below never jumps.
func TestTheCornerNeverChangesWidth(t *testing.T) {
	widths := map[int]bool{}
	m := healthyModel()
	widths[len([]rune(m.statePill()))] = true

	m.platform = api.PlatformStatus{Indicator: "critical"}
	widths[len([]rune(m.statePill()))] = true
	m.err = errFake{}
	widths[len([]rune(m.statePill()))] = true
	m.err, m.sseLive = nil, false
	widths[len([]rune(m.statePill()))] = true

	if len(widths) != 1 {
		t.Errorf("the corner changed width across states: %v", widths)
	}
}

// i opens it from any tab, i or esc closes it.
func TestTheStateModalOpensAndClosesOnItsOwnKeys(t *testing.T) {
	for _, tb := range []tab{tabSessions, tabStats, tabMachines, tabSettings} {
		m := healthyModel()
		m.tab = tb
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(stateKey)})
		got := next.(model)
		if !got.showState {
			t.Fatalf("tab %v: %q did not open the state modal", tb, stateKey)
		}
		if !strings.Contains(got.View(), "State") {
			t.Errorf("tab %v: the modal is open but not drawn:\n%s", tb, got.View())
		}
		closed, _ := got.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		if closed.(model).showState {
			t.Errorf("tab %v: esc did not close the modal", tb)
		}
	}
}

// The two indicators that left the bottom bar must be findable in the modal, or
// this is a deletion rather than a move.
func TestThePlatformAndFreshnessMovedIntoTheModal(t *testing.T) {
	m := healthyModel()
	if strings.Contains(m.bottomBar(), "platform") || strings.Contains(m.bottomBar(), "⟳") {
		t.Errorf("the bottom bar still carries a reliability indicator:\n%s", m.bottomBar())
	}
	out := renderState(m.stateRows(), 200)
	for _, want := range []string{"claude platform", "usage snapshot", "vigie server", "watcher", "client / daemon"} {
		if !strings.Contains(out, want) {
			t.Errorf("the state modal is missing %q:\n%s", want, out)
		}
	}
}

// `platform` alone reads as though it were about vigie; it is Claude's
// Statuspage, polled by the server (ADR-0005).
func TestThePlatformRowNamesClaude(t *testing.T) {
	if r := rowsByLabel(healthyModel())["claude platform"]; r.label != "claude platform" {
		t.Errorf("the platform row is labeled %q", r.label)
	}
}
