package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #449. A panel keeps its figures when a refresh fails — blanking on a transient
// blip would be worse — but it used to present them as current, so a panel that
// had been failing for an hour looked exactly like one that was up to date.

const staleWord = "could not refresh"

func failingModel(t *testing.T, source string) model {
	t.Helper()
	m := stubModel()
	m.width = 120
	switch source {
	case srcStats:
		m.fetchStats = func() (api.StatsResponse, error) { return api.StatsResponse{}, errFetch }
	case srcSettings:
		m.fetchSettings = func() (api.Settings, error) { return api.Settings{}, errFetch }
	case srcWatcher:
		m.fetchWatcher = func() (api.WatcherStatus, error) { return api.WatcherStatus{}, errFetch }
	case srcUsage:
		m.fetchUsage = func() (api.UsageReport, error) { return api.UsageReport{}, errFetch }
	case srcVersion:
		m.fetchVersion = func() (api.VersionInfo, error) { return api.VersionInfo{}, errFetch }
	default:
		t.Fatalf("unknown source %q", source)
	}
	return m
}

func TestStatsPanelSaysItCouldNotRefresh(t *testing.T) {
	m := stubModel()
	m.width = 120
	m = m.applyDataMsg(m.statsCmd()()) // a good refresh first
	if strings.Contains(m.renderStats(), staleWord) {
		t.Fatal("a healthy panel is flagged as stale")
	}

	m.fetchStats = func() (api.StatsResponse, error) { return api.StatsResponse{}, errFetch }
	m = m.applyDataMsg(m.statsCmd()())

	out := m.renderStats()
	if !strings.Contains(out, staleWord) {
		t.Errorf("the stats panel does not say the refresh failed:\n%s", out)
	}
	if m.stats.SessionCount != 3 {
		t.Errorf("the figures were dropped: %+v", m.stats)
	}
}

// Recovery must clear the notice, or it becomes noise nobody reads.
func TestTheNoticeClearsOnRecovery(t *testing.T) {
	m := failingModel(t, srcStats)
	m = m.applyDataMsg(m.statsCmd()())
	if !strings.Contains(m.renderStats(), staleWord) {
		t.Fatal("the failure was not flagged")
	}

	m.fetchStats = func() (api.StatsResponse, error) { return api.StatsResponse{SessionCount: 7}, nil }
	m = m.applyDataMsg(m.statsCmd()())

	if strings.Contains(m.renderStats(), staleWord) {
		t.Error("the notice survived a successful refresh")
	}
}

func TestSettingsPanelCoversItsOwnSources(t *testing.T) {
	for _, src := range []string{srcSettings, srcVersion} {
		m := failingModel(t, src)
		switch src {
		case srcSettings:
			m = m.applyDataMsg(m.settingsCmd()())
		case srcVersion:
			m = m.applyDataMsg(m.fetchVersionCmd()())
		}
		if out := m.renderSettings(); !strings.Contains(out, staleWord) {
			t.Errorf("%s: the settings panel says nothing", src)
		}
	}
}

// The bottom bar used to append a bare warning glyph beside the usage gauges when
// their refresh failed. It went with #726: the state pill turns amber on every
// tab and the state modal's usage row says strictly more — how old the snapshot
// is, and that it cannot refresh. This test used to assert the glyph appeared;
// what it asserts now is that the fact still reaches the operator, and no longer
// twice.
//
// The criterion is #651's: a Sessions-tab warning goes when the modal has a row
// for its source, and stays when it does not — which is why the notes on Stats,
// Settings and Machines are untouched.
func TestAFailingUsageRefreshIsSaidOnceNotTwice(t *testing.T) {
	m := failingModel(t, srcUsage)
	m = m.applyDataMsg(m.fetchUsageCmd()())
	m.width = 120

	if strings.Contains(m.bottomBar(), "⚠") {
		t.Error("the bottom bar still marks the gauges; the modal already carries it")
	}
	if m.stateLevel() == levelOK {
		t.Error("the state pill reads healthy while the usage snapshot cannot refresh")
	}
}

// Each source is tracked on its own: one failing endpoint must not flag panels
// fed by others.
func TestSourcesAreFlaggedIndependently(t *testing.T) {
	m := failingModel(t, srcStats)
	m = m.applyDataMsg(m.statsCmd()())
	m = m.applyDataMsg(m.settingsCmd()())
	m = m.applyDataMsg(m.fetchVersionCmd()())

	if !strings.Contains(m.renderStats(), staleWord) {
		t.Error("the failing stats panel is not flagged")
	}
	if strings.Contains(m.renderSettings(), staleWord) {
		t.Error("a healthy settings panel was flagged by another source's failure")
	}
}

// This test used to assert the opposite: that sessions stayed out of the stale
// tracking because `m.err` already reached the operator. #456 reversed that —
// `m.err` was not reporting the failure, it was standing in for the table. The
// failure now surfaces in both places: `m.err` carries the reason, the tracking
// puts the sessions row of the state modal into fault, and the table stays. The
// note the Sessions tab used to draw beside them went with #650 — it said the
// same thing, one keystroke closer and for as long as the outage lasted.
func TestSessionsFailureKeepsTheTableAndSaysWhy(t *testing.T) {
	m := stubModel()
	m.width = 120
	m.fetch = func() ([]api.SessionView, error) { return nil, errFetch }
	m = m.applySessions(m.fetchCmd(1)().(sessionsMsg))

	if m.err == nil {
		t.Fatal("the sessions failure no longer surfaces")
	}
	if !m.refreshFailed[srcSessions] {
		t.Error("the failure is not tracked, so no note would be drawn")
	}
}
