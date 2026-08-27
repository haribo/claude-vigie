package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
)

// These cases came from internal/tui when the rules did (ADR-0011, #618). They
// are the same cases: what moved is where the answer is computed, not what the
// answer is.

func TestASessionIsNamedByItsTitleAndFallsBackToTheShortID(t *testing.T) {
	if got := nameView("my-conv", "5c483c16-x"); got != "my-conv" {
		t.Errorf("nameView with a title = %q, want my-conv", got)
	}
	if got := nameView("", "5c483c16-x"); got != "5c483c16" {
		t.Errorf("nameView without a title = %q, want the short id 5c483c16", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID of an id shorter than the cut = %q, want abc", got)
	}
}

// The GNOME indicator fell back to the project directory where the other two
// clients fell back to the short id, so one untitled session had two names
// depending on which window the operator looked at (#618).
//
// This proves the daemon's half only. The client half is TestNoClientDerivesAName
// below — a test over nameView alone would have stayed green with extension.js
// untouched, which is the whole reason the divergence lasted.
func TestAnUntitledSessionIsNamedByItsShortIDNotItsDirectory(t *testing.T) {
	got := nameView("", "5c483c16-96b5-4f61")
	if got == projectView("/home/x/example-app") {
		t.Fatal("the name fell back to the directory — the GNOME divergence is back")
	}
	if got != "5c483c16" {
		t.Errorf("nameView = %q, want 5c483c16", got)
	}
}

// The rule is only settled if no client still holds one. The GNOME indicator
// named a session `s.title || basename(s.project_dir) || s.id` and nothing could
// report it: it has no test suite of its own, which ADR-0011 names as the reason
// it is fixed by moving the rule rather than by checking the copy.
//
// So the check is over the sources, the way internal/status guards the status
// vocabulary against these same two files. A directory basename reappearing in
// either client is the divergence coming back.
func TestNoClientDerivesAName(t *testing.T) {
	for _, c := range []struct {
		path    string
		banned  string
		why     string
		require string
	}{
		{"../../gnome-extension/extension.js", "basename(",
			"the GNOME indicator named an untitled session after its project directory (#618)", "s.name"},
		{"../../gnome-extension/lib.js", "basename(",
			"the helper the indicator used to do it", ""},
		{"../../internal/web/static/lib.js", "projectName(",
			"the dashboard's twin of the same rule", ""},
	} {
		b, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("reading %s: %v — the extraction is broken, not the code", c.path, err)
		}
		src := string(b)
		if strings.Contains(src, c.banned) {
			t.Errorf("%s still calls %s — %s. The daemon sends `name`; a client that derives one can disagree with it",
				c.path, c.banned, c.why)
		}
		if c.require != "" && !strings.Contains(src, c.require) {
			t.Errorf("%s does not read %s — it must render the name the daemon sent", c.path, c.require)
		}
	}
}

func TestTheProjectIsTheLastSegmentOrADash(t *testing.T) {
	if got := projectView("/home/x/example-app"); got != "example-app" {
		t.Errorf("projectView = %q, want example-app", got)
	}
	if got := projectView(""); got != "-" {
		t.Errorf("projectView of no directory = %q, want -", got)
	}
}

func TestThePermissionModeTaxonomy(t *testing.T) {
	labels := map[string]string{
		"default": "manual", "acceptEdits": "accept", "plan": "plan",
		"auto": "auto", "bypassPermissions": "bypass", "": "-",
		"someNewMode": "someNewMode", // unknown surfaced raw, never faked as "manual" (#304)
	}
	for raw, want := range labels {
		if got := modeLabelView(raw); got != want {
			t.Errorf("modeLabelView(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := modeDetailView("plan"); got != "plan — awaiting plan approval" {
		t.Errorf("modeDetailView(plan) = %q", got)
	}
	if got := modeDetailView("someNewMode"); got != "someNewMode" {
		t.Errorf("modeDetailView of an unknown mode = %q, want it raw", got)
	}
}

// The web dashboard holds its own copy of this taxonomy, and
// `test/js/dashboard.test.mjs` reads the same fixture. A case added to one side
// and forgotten on the other fails here (#550).
func TestModeLabelsAgreeWithTheSharedFixture(t *testing.T) {
	var f struct {
		Mode []struct {
			Why  string `json:"why"`
			Raw  string `json:"raw"`
			Want string `json:"want"`
		} `json:"mode"`
	}
	b, err := os.ReadFile("../../test/fixtures/column-cases.json")
	if err != nil {
		t.Fatalf("reading the shared fixture: %v", err)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parsing the shared fixture: %v", err)
	}
	if len(f.Mode) == 0 {
		t.Fatal("the shared fixture has no mode cases — the extraction is broken, not the code")
	}
	for _, c := range f.Mode {
		if got := modeLabelView(c.Raw); got != c.Want {
			t.Errorf("modeLabelView(%q) = %q, want %q — %s", c.Raw, got, c.Want, c.Why)
		}
	}
}

// Both clients name an API error, and architecture.md binds them on content —
// "A divergence in content is debt, not design". Two hand-kept lists of one
// vocabulary is how that debt is taken on, so both sides read this fixture (#584).
func TestAPIErrorLabelsMatchTheSharedFixture(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Code  int    `json:"code"`
			Label string `json:"label"`
		} `json:"cases"`
	}
	body, err := os.ReadFile("../../test/fixtures/api-error-labels.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("the shared fixture has no cases — the extraction is broken, not the code")
	}
	for _, c := range fixture.Cases {
		if got := apiErrorLabel(c.Code); got != c.Label {
			t.Errorf("apiErrorLabel(%d) = %q, the shared fixture says %q", c.Code, got, c.Label)
		}
	}
}

func TestTheDetailPrecedence(t *testing.T) {
	cases := []struct {
		why       string
		s         store.Session
		effective string
		want      string
	}{
		{"a call takes the cell, message and all (#389)",
			store.Session{CallAt: "t", CallMessage: "backfill done", Detail: "Edit render.go"}, "working", "backfill done"},
		{"a call with no message still takes it",
			store.Session{CallAt: "t", Detail: "Edit render.go"}, "working", "called you"},
		{"an API error outranks the last tool (#584)",
			store.Session{APIErrorStatus: 529, Detail: "Edit render.go"}, "error", "529 Overloaded"},
		{"an unnamed code is still shown",
			store.Session{APIErrorStatus: 503}, "error", "503"},
		{"an error code on a session that is not in error says nothing",
			store.Session{APIErrorStatus: 529, Detail: "Edit render.go"}, "working", "Edit render.go"},
		{"otherwise the stored detail", store.Session{Detail: "Edit render.go"}, "working", "Edit render.go"},
		{"and a dash when there is none", store.Session{}, "idle", "-"},
	}
	for _, c := range cases {
		if got := detailTextView(c.s, c.effective); got != c.want {
			t.Errorf("detailTextView = %q, want %q — %s", got, c.want, c.why)
		}
	}
}

// A session whose reports went stale is shown under a status the store does not
// hold. A derived field read from the stored one would describe a session the
// client is never shown — the #617 mistake, in a new field.
func TestTheDetailReadsTheEffectiveStatusNotTheStoredOne(t *testing.T) {
	s := store.Session{Status: "error", APIErrorStatus: 529, Detail: "Edit render.go"}
	if got := detailTextView(s, "ended"); got != "Edit render.go" {
		t.Errorf("detailTextView on an ended session = %q, want the detail — "+
			"the stored `error` must not resurrect the code", got)
	}
}

// The Stats tab's ranking used to name an untitled session by its whole id while
// the sessions table named it by the first eight characters, so the same session
// answered to two names one tab apart (#630).
func TestTheStatsRankingNamesASessionLikeTheTableDoes(t *testing.T) {
	top := topSessions([]store.Session{{ID: "a1b2c3d4-96b5-4f61-b09e-deadbeef", Usage: store.Usage{OutputTokens: 1}}})
	if len(top) != 1 {
		t.Fatalf("topSessions returned %d rows, want 1", len(top))
	}
	// The literal, not nameView(...) again: comparing the ranking against the very
	// function it calls would pass whatever either of them did.
	if top[0].Name != "a1b2c3d4" {
		t.Errorf("ranking name = %q, want a1b2c3d4 — the table shows the short id", top[0].Name)
	}
}
