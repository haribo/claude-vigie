package tui

import (
	"errors"
	"strings"
	"testing"

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
			{ID: "s1", Title: "api-gateway", Status: "working"},
			{ID: "s2", Title: "web-app", Status: "waiting"},
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
// token both blank a screen, but they call for different actions.
func TestTheFailureIsStillAnnouncedWithItsReason(t *testing.T) {
	m := twoSessions(t)
	m.fetch = func() ([]api.SessionView, error) { return nil, errBlip }
	m = m.applySessions(m.fetchCmd(2)().(sessionsMsg))

	out := m.viewSessions()
	if !strings.Contains(out, "could not refresh") {
		t.Errorf("nothing says the figures are not current:\n%s", out)
	}
	if !strings.Contains(out, "context deadline exceeded") {
		t.Errorf("the reason is not shown:\n%s", out)
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
	if !strings.Contains(m.viewSessions(), "could not refresh") {
		t.Fatal("the failure was not announced")
	}

	m.fetch = func() ([]api.SessionView, error) {
		return []api.SessionView{{ID: "s1", Title: "api-gateway", Status: "idle"}}, nil
	}
	m = m.applySessions(m.fetchCmd(3)().(sessionsMsg))

	out := m.viewSessions()
	if strings.Contains(out, "could not refresh") {
		t.Errorf("the notice survived a successful poll:\n%s", out)
	}
	if !strings.Contains(out, "api-gateway") {
		t.Errorf("the recovered table is missing:\n%s", out)
	}
}

// A healthy model shows no notice at all — the check that keeps this from
// becoming decoration nobody reads.
func TestAHealthyTableCarriesNoNotice(t *testing.T) {
	if out := twoSessions(t).viewSessions(); strings.Contains(out, "could not refresh") {
		t.Errorf("a healthy table is flagged:\n%s", out)
	}
}
