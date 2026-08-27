package tui

import (
	"errors"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// The model already takes its data sources as injected function fields, set in
// tui.go from the real fetchers. No test had ever substituted one, so the whole
// refresh cycle — command, message, fold into the model — was uncovered (#445).
//
// What is worth checking is not each three-line closure but the cycle: that a
// refresh reaches the view, that a failure does not blank what is already there,
// and that a late answer cannot overwrite a newer one.

var errFetch = errors.New("server unreachable")

// stubModel returns a model wired to fixed answers, the way tui.go wires it to
// real ones.
func stubModel() model {
	return model{
		fetch:      func() ([]api.SessionView, error) { return []api.SessionView{{ID: "s1", Status: "working"}}, nil },
		fetchUsage: func() (api.UsageReport, error) { return api.UsageReport{FiveHourPct: 47}, nil },
		fetchWatcher: func() (api.WatcherStatus, error) {
			return api.WatcherStatus{Machines: map[string]string{"orion": "2026-08-14T10:00:00Z"}}, nil
		},
		fetchSettings: func() (api.Settings, error) { return api.Settings{SessionRetention: "48h"}, nil },
		fetchStats:    func() (api.StatsResponse, error) { return api.StatsResponse{SessionCount: 3}, nil },
		fetchPlatform: func() (api.PlatformStatus, error) { return api.PlatformStatus{Indicator: "none"}, nil },
		fetchVersion:  func() (api.VersionInfo, error) { return api.VersionInfo{Version: "9.9.9"}, nil },
		sess:          sessionsView{prevStatus: map[string]string{}, prevCall: map[string]bool{}},
	}
}

func TestInitStartsTheRefresh(t *testing.T) {
	if cmd := stubModel().Init(); cmd == nil {
		t.Fatal("Init returned no command — nothing would ever load")
	}
}

// Each command must call *its own* injected fetcher and wrap the answer in the
// message the model folds. A crossed wire here would show one endpoint's data
// under another's heading.
func TestEachCommandCarriesItsOwnAnswer(t *testing.T) {
	m := stubModel()

	if msg, ok := m.fetchCmd(1)().(sessionsMsg); !ok || len(msg.sessions) != 1 || msg.sessions[0].ID != "s1" {
		t.Errorf("fetchCmd produced %#v", msg)
	}
	if msg, ok := m.fetchUsageCmd()().(usageMsg); !ok || msg.usage.FiveHourPct != 47 {
		t.Errorf("fetchUsageCmd produced %#v", msg)
	}
	if msg, ok := m.fetchPlatformCmd()().(platformMsg); !ok || msg.ps.Indicator != "none" {
		t.Errorf("fetchPlatformCmd produced %#v", msg)
	}
	if msg, ok := m.fetchVersionCmd()().(versionMsg); !ok || msg.v.Version != "9.9.9" {
		t.Errorf("fetchVersionCmd produced %#v", msg)
	}
	if msg, ok := m.statsCmd()().(statsMsg); !ok || msg.stats.SessionCount != 3 {
		t.Errorf("statsCmd produced %#v", msg)
	}
	if msg, ok := m.settingsCmd()().(settingsMsg); !ok || msg.retention != "48h" {
		t.Errorf("settingsCmd produced %#v", msg)
	}
	if msg, ok := m.watcherCmd()().(watcherMsg); !ok || msg.status.Machines["orion"] != "2026-08-14T10:00:00Z" {
		t.Errorf("watcherCmd produced %#v", msg)
	}
}

// A fetched answer has to reach the model, or the refresh is decorative.
func TestAnAnswerReachesTheModel(t *testing.T) {
	m := stubModel()

	m = m.applySessions(m.fetchCmd(1)().(sessionsMsg))
	if len(m.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(m.sessions))
	}
	m = m.applyDataMsg(m.fetchUsageCmd()())
	m = m.applyDataMsg(m.statsCmd()())
	m = m.applyDataMsg(m.fetchVersionCmd()())

	if m.usage.FiveHourPct != 47 {
		t.Errorf("usage not folded in: %+v", m.usage)
	}
	if m.stats.SessionCount != 3 {
		t.Errorf("stats not folded in: %+v", m.stats)
	}
	if m.daemonVersion.Version != "9.9.9" {
		t.Errorf("daemon version not folded in: %+v", m.daemonVersion)
	}
}

// A failing fetch must not blank what the operator is already looking at. This is
// the behavior that matters when a daemon restarts under a live dashboard.
func TestAFailedFetchKeepsWhatIsAlreadyThere(t *testing.T) {
	m := stubModel()
	m = m.applySessions(m.fetchCmd(1)().(sessionsMsg))
	m = m.applyDataMsg(m.statsCmd()())

	m.fetch = func() ([]api.SessionView, error) { return nil, errFetch }
	m.fetchStats = func() (api.StatsResponse, error) { return api.StatsResponse{}, errFetch }

	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))
	m = m.applyDataMsg(m.statsCmd()())

	if len(m.sessions) != 1 {
		t.Errorf("sessions wiped by a failed fetch: %d left", len(m.sessions))
	}
	if m.stats.SessionCount != 3 {
		t.Errorf("stats wiped by a failed fetch: %+v", m.stats)
	}
	if !errors.Is(m.err, errFetch) {
		t.Errorf("m.err = %v, want the sessions failure surfaced", m.err)
	}
}

// A recovered fetch clears the error, so a transient outage does not leave a
// stale message on screen.
func TestARecoveredFetchClearsTheError(t *testing.T) {
	m := stubModel()
	m.fetch = func() ([]api.SessionView, error) { return nil, errFetch }
	m = m.applySessions(m.fetchCmd(1)().(sessionsMsg))
	if m.err == nil {
		t.Fatal("the failure did not surface")
	}

	m.fetch = func() ([]api.SessionView, error) { return []api.SessionView{{ID: "s2"}}, nil }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))
	if m.err != nil {
		t.Errorf("m.err = %v after recovery, want nil", m.err)
	}
}

// Responses can arrive out of order. A late answer to an older request must not
// overwrite a newer one — otherwise the table flickers back to stale rows.
func TestALateAnswerCannotOverwriteANewerOne(t *testing.T) {
	m := stubModel()

	m.fetch = func() ([]api.SessionView, error) { return []api.SessionView{{ID: "old"}}, nil }
	stale := m.fetchCmd(1)().(sessionsMsg)

	m.fetch = func() ([]api.SessionView, error) { return []api.SessionView{{ID: "new"}}, nil }
	fresh := m.fetchCmd(2)().(sessionsMsg)

	m = m.applySessions(fresh)
	m = m.applySessions(stale) // arrives late

	if len(m.sessions) != 1 || m.sessions[0].ID != "new" {
		t.Errorf("sessions = %+v, want the newer answer kept", m.sessions)
	}
}

// refreshSessions bumps the generation, which is what makes the check above
// meaningful: two refreshes must not share a generation.
func TestRefreshSessionsBumpsTheGeneration(t *testing.T) {
	m := stubModel()
	m.refreshSessions()
	first := m.fetchSeq
	m.refreshSessions()

	if m.fetchSeq <= first {
		t.Errorf("generation went %d then %d, want it to advance", first, m.fetchSeq)
	}
}

// A model with no fetcher wired must produce no command rather than panic. Three
// of the factories guard for this; the guard is what lets a partially wired model
// exist at all.
func TestAbsentFetchersProduceNoCommand(t *testing.T) {
	var m model
	if m.fetchPlatformCmd() != nil {
		t.Error("fetchPlatformCmd returned a command with no fetcher")
	}
	if m.fetchVersionCmd() != nil {
		t.Error("fetchVersionCmd returned a command with no fetcher")
	}
	if m.watcherCmd() != nil {
		t.Error("watcherCmd returned a command with no fetcher")
	}
	if m.statsCmd() != nil {
		t.Error("statsCmd returned a command with no fetcher")
	}
	if m.settingsCmd() != nil {
		t.Error("settingsCmd returned a command with no fetcher")
	}
}
