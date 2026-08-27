package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #546. The dashboard groups the table too, and it has its own copy of the rule
// because the browser cannot import Go. The mode names matter as much as the
// behavior: an operator's saved preference holds the *name* on both sides, so a
// mode renamed on one client and not the other silently resets their grouping.
//
// This used to read the shipped `lib.js` with a regular expression. That checked a
// constant array matched and could say nothing about what either side does at a
// boundary, which is where this repository's rules have actually diverged (#619).
// Both suites read one case list instead. That retires `jsArrayFromFile`, which is
// half of ADR-0011's measurable end state; the other half — "no Go test reads a
// JavaScript file to check a constant" — is not reached, and saying so is the
// point. `internal/status/status_test.go:63` still holds a character-for-character
// clone of the same helper, because the constants it checks (`STATUSES` in app.js,
// `STATUS_ORDER` in extension.js) are module-private: no test suite can import
// them, so a shared case list cannot reach them until they move to a lib. That is
// tracked separately, not silently left (#633).

type groupFixture struct {
	Modes []string `json:"modes"`
	Keys  []struct {
		Why     string `json:"why"`
		Mode    string `json:"mode"`
		Machine string `json:"machine"`
		Project string `json:"project"`
		Want    string `json:"want"`
	} `json:"keys"`
}

func loadGroupFixture(t *testing.T) groupFixture {
	t.Helper()
	f := loadFixture[groupFixture](t, "group-cases.json")
	if len(f.Modes) == 0 || len(f.Keys) == 0 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	return f
}

func TestTheGroupModesAgreeWithTheSharedFixture(t *testing.T) {
	f := loadGroupFixture(t)
	var got []string
	for g := groupNone; g < groupByCount; g++ {
		n, ok := groupNames[g]
		if !ok {
			t.Fatalf("groupNames has no name for mode %d — the enum and the map disagree", g)
		}
		got = append(got, n)
	}
	if len(got) != len(f.Modes) {
		t.Fatalf("groupNames = %v, the shared fixture says %v", got, f.Modes)
	}
	for i, want := range f.Modes {
		if got[i] != want {
			t.Errorf("mode %d = %q, the shared fixture says %q — a renamed mode resets a saved preference", i, got[i], want)
		}
	}
}

func TestGroupKeyAgreesWithTheSharedFixture(t *testing.T) {
	byName := map[string]groupBy{}
	for g := groupNone; g < groupByCount; g++ {
		byName[groupNames[g]] = g
	}
	for _, c := range loadGroupFixture(t).Keys {
		mode, ok := byName[c.Mode]
		if !ok {
			t.Fatalf("the fixture names a mode %q this build does not have", c.Mode)
		}
		s := api.SessionView{Machine: c.Machine, Project: c.Project}
		if got := groupKey(s, mode); got != c.Want {
			t.Errorf("groupKey(%q) = %q, want %q — %s", c.Mode, got, c.Want, c.Why)
		}
	}
}
