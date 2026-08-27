package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #545. The dashboard filters with its own copy of `fuzzyMatch`, because the TUI
// is Go and the browser is not. A rule copied per consumer is precisely what
// #421, #422 and #466 were, and the copy that drifts is never the one anybody is
// looking at.
//
// So neither implementation is trusted on its word. This test and
// `test/js/dashboard.test.mjs` read the same fixture and must return the same
// verdict for every case. A change to either implementation that is not a change
// to the other fails on one side or the other.
//
// What this does NOT cover: the haystack. Field order and the joining spaces
// matter — a pattern may span two fields — but comparing them would mean building
// an `api.SessionView` in JSON and rendering it on both sides, which is a heavier
// harness than the risk warrants today. The haystack is asserted separately in
// each language against the same hand-written example.

type fuzzyCase struct {
	Why     string `json:"why"`
	Pattern string `json:"pattern"`
	Text    string `json:"text"`
	Want    bool   `json:"want"`
}

func loadFuzzyCases(t *testing.T) []fuzzyCase {
	t.Helper()
	doc := loadFixture[struct {
		Cases []fuzzyCase `json:"cases"`
	}](t, "fuzzy-cases.json")
	if len(doc.Cases) == 0 {
		t.Fatal("the shared fixture has no cases — the extraction is broken, not the code")
	}
	return doc.Cases
}

func TestFuzzyMatchAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadFuzzyCases(t) {
		if got := fuzzyMatch(c.Pattern, c.Text); got != c.Want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v — %s", c.Pattern, c.Text, got, c.Want, c.Why)
		}
	}
}

// The haystack the filter searches: field order and the single spaces are part of
// the rule, since a pattern may run from the end of one field into the start of
// the next. The dashboard asserts the same example against its own twin.
func TestSessionHaystackShape(t *testing.T) {
	s := stubSessionForHaystack()
	const want = "api-gateway orion-dev gateway main working"
	if got := sessionHaystack(s); got != want {
		t.Errorf("sessionHaystack = %q, want %q", got, want)
	}
}

// An untitled session is named by the first eight characters of its id, so that
// is what the filter can reach — not the whole id. The eight-character cut is the
// daemon's (nameView) and is proved there; what this guards is that the haystack
// searches the *name* and never reaches past it into the raw id.
func TestAnUntitledSessionIsSearchableByItsShortId(t *testing.T) {
	s := stubSessionForHaystack()
	s.Title = ""
	s.ID = "abcdefghij-the-rest-is-unreachable"
	s.Name = "abcdefgh"
	got := sessionHaystack(s)
	if want := "abcdefgh "; got[:len(want)] != want {
		t.Errorf("haystack starts %q, want %q", got[:len(want)], want)
	}
	if fuzzyMatch("abcdefghij", got) {
		t.Error("the ninth character of the id is searchable — the name shows eight")
	}
}

func stubSessionForHaystack() api.SessionView {
	return api.SessionView{
		// Name and Project as the daemon derives them (ADR-0011, #618); the raw
		// fields beside them are what it derived them from.
		Title: "api-gateway", Name: "api-gateway", Machine: "orion-dev",
		ProjectDir: "/home/ada/gateway", Project: "gateway",
		GitBranch: "main", Status: "working",
	}
}
