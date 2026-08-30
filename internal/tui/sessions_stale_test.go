package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// #456, observed on a laptop resuming from suspend: one failed poll replaced the
// whole table with a single error line, and the fleet disappeared for the minutes
// it took the connection to come back. The sessions were never lost — the model
// kept them — so this was the view throwing away data it still had.
//
// It was also the last panel doing what #454 fixed everywhere else.

// The shape a resumed laptop actually produces. Lower-cased to satisfy ST1005;
// the wording that matters for the test is the tail.
var errBlip = errors.New(`get "https://example/api/sessions": context deadline exceeded`)

// twoSessions is a model that has successfully loaded, the state a resumed laptop
// is in before its first failed poll.
func twoSessions(t *testing.T) model {
	t.Helper()
	m := stubModel()
	m.width = 140
	m.fetch = func() ([]api.SessionView, error) {
		return []api.SessionView{
			{ID: "s1", Title: "api-gateway", Name: "api-gateway", Status: "working"},
			{ID: "s2", Title: "web-app", Name: "web-app", Status: "waiting"},
		}, nil
	}
	return m.applySessions(m.fetchCmd(1)().(sessionsMsg))
}

func TestTheTableSurvivesAFailedPoll(t *testing.T) {
	m := twoSessions(t)
	if out := m.viewSessions(); !strings.Contains(out, "api-gateway") {
		t.Fatalf("the table did not render to begin with:\n%s", out)
	}

	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))

	out := m.viewSessions()
	if !strings.Contains(out, "api-gateway") || !strings.Contains(out, "web-app") {
		t.Errorf("a failed poll blanked the fleet:\n%s", out)
	}
}

// The operator must still be told, and told *why* — a network blip and a rejected
// token both blank a screen, but they call for different actions. Since #650 that
// happens in the state modal rather than in a banner over the table: the banner
// said the same thing less precisely and stayed up for as long as the outage.
func TestTheFailureIsStillAnnouncedWithItsReason(t *testing.T) {
	m := twoSessions(t)
	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))

	if m.serverRow().level != levelBroken {
		t.Errorf("a failed poll left the server row at %v — nothing says the figures are old", m.serverRow())
	}
	modal := renderState(m.stateRows(), 140)
	if !strings.Contains(modal, "context deadline exceeded") {
		t.Errorf("the modal does not name the reason:\n%s", modal)
	}
	if out := m.viewSessions(); !strings.Contains(out, "api-gateway") {
		t.Errorf("the fleet went missing with the banner:\n%s", out)
	}
}

// With nothing to fall back on, the error is all there is to show — no empty
// table pretending to be a fleet.
func TestTheErrorStandsAloneOnlyWithNothingToShow(t *testing.T) {
	m := stubModel()
	m.width = 140
	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(1)().(sessionsMsg))

	out := m.viewSessions()
	if !strings.Contains(out, "context deadline exceeded") {
		t.Errorf("a first-poll failure says nothing:\n%s", out)
	}
	if strings.Contains(out, "no sessions yet") {
		t.Errorf("a failure was reported as an empty fleet:\n%s", out)
	}
}

// Recovery clears the notice, so a blip does not leave a warning up forever.
func TestTheNoticeGoesWhenThePollRecovers(t *testing.T) {
	m := twoSessions(t)
	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))
	if m.serverRow().level != levelBroken {
		t.Fatal("the failure was not announced")
	}

	m.fetch = func() ([]api.SessionView, error) {
		return []api.SessionView{{ID: "s1", Title: "api-gateway", Name: "api-gateway", Status: "idle"}}, nil
	}
	m = m.applySessions(m.fetchCmd(3)().(sessionsMsg))

	if m.serverRow().level == levelBroken {
		t.Errorf("the alarm survived a successful poll: %v", m.serverRow())
	}
	if out := m.viewSessions(); !strings.Contains(out, "api-gateway") {
		t.Errorf("the recovered table is missing:\n%s", out)
	}
}

// A healthy model flags nothing — the check that keeps the pill from becoming
// decoration nobody reads.
func TestAHealthyTableCarriesNoNotice(t *testing.T) {
	m := twoSessions(t)
	if m.serverRow().level == levelBroken {
		t.Errorf("a healthy fleet is flagged: %v", m.serverRow())
	}
}

// The two banners this tab used to carry are gone for good (#650): the pill takes
// the worst level on every tab and the modal names the fault, and for the watcher
// it names the machine, which the banner never did.
//
// Each is asserted in the state that *used to draw it* — a healthy model would
// pass however the code behaved, since both lines were conditional — and over
// `View()`, because the watcher banner was drawn there and not in `viewSessions`.
//
// The two faults are exercised apart on purpose. With the link down the modal
// answers `unknown` for the watcher rather than naming a machine, because a TUI
// that cannot reach the server cannot know which watchers are reporting (§ 4 of
// docs/design/sessions-chrome.md). The banner asserted "no watcher reporting" in
// that state anyway, which was not a fact it had.
func TestTheSessionsTabDrawsNoWarningLineWhenARefreshFails(t *testing.T) {
	m := twoSessions(t)
	m.height = 40
	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))
	if !m.refreshFailed[srcSessions] || m.err == nil {
		t.Fatal("the fixture is not in the failed-refresh state — the check would prove nothing")
	}

	if out := m.View(); strings.Contains(out, "could not refresh") {
		t.Errorf("the refresh banner is back on the sessions tab:\n%s", out)
	}
	if modal := renderState(m.stateRows(), 140); !strings.Contains(modal, "context deadline exceeded") {
		t.Errorf("the modal lost the refresh failure:\n%s", modal)
	}
}

func TestTheSessionsTabDrawsNoWarningLineWhenAWatcherStops(t *testing.T) {
	m := twoSessions(t)
	m.height = 40
	m.gotWatcher = true
	m.watcherMachines = map[string]string{"orion": "2020-01-01T00:00:00Z", "beta": m.now().Format(time.RFC3339)}
	if alarm, _, _, _ := fleetAlarm(m.watcherMachines, m.now()); !alarm {
		t.Fatal("the fixture's watcher is not stale — the check would prove nothing")
	}

	if out := m.View(); strings.Contains(out, "no watcher reporting") {
		t.Errorf("the watcher banner is back on the sessions tab:\n%s", out)
	}
	// And says more than the banner did: which machine, and how many of how many.
	modal := renderState(m.stateRows(), 140)
	if !strings.Contains(modal, "orion") {
		t.Errorf("the modal does not name the silent machine:\n%s", modal)
	}
}
