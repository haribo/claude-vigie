package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #550. The dashboard renders CTX and MODE now, which meant copying two rules
// into JavaScript: how full a context window is, and the #303 permission-mode
// taxonomy. A rule copied per consumer is what #421, #422 and #466 were, so
// neither copy is trusted — this and `test/js/dashboard.test.mjs` read the same
// fixture and must return the same string for every case.

type columnFixture struct {
	Context []struct {
		Why    string `json:"why"`
		Model  string `json:"model"`
		Tokens *int64 `json:"tokens"` // nil is "no reading at all", which is not zero (#367)
		Pct    *int   `json:"pct"`    // the daemon's derived reading; nil exactly when Tokens is
		Want   string `json:"want"`
	} `json:"context"`
	Mode []struct {
		Why  string `json:"why"`
		Raw  string `json:"raw"`
		Want string `json:"want"`
	} `json:"mode"`
}

func loadColumnFixture(t *testing.T) columnFixture {
	t.Helper()
	f := loadFixture[columnFixture](t, "column-cases.json")
	// Only the context half is read here: the mode labels moved to the daemon with
	// the rest of the naming family, and internal/server/naming_test.go reads the
	// mode section of this same fixture (ADR-0011, #618).
	if len(f.Context) == 0 {
		t.Fatal("the shared fixture has no context cases — the extraction is broken, not the code")
	}
	return f
}

func TestContextCellAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadColumnFixture(t).Context {
		// The daemon derives Pct from the model and the reading (ADR-0011); this
		// layer is asked only what it renders from the answer, which is exactly the
		// half the dashboard still holds a copy of.
		s := api.SessionView{Model: c.Model, ContextTokens: c.Tokens, ContextPct: c.Pct}
		if got := contextCell(s); got != c.Want {
			t.Errorf("contextCell(pct=%v) = %q, want %q — %s", c.Pct, got, c.Want, c.Why)
		}
	}
}

// webExtraColumns are keys the dashboard has and the TUI cannot: a pointer-driven
// affordance is a *gesture*, free under #544. Everything else must match.
var webExtraColumns = map[string]bool{"open": true}

// TestDashboardSharesTheColumnSet is the guard #550 exists to make possible. The
// two sets had drifted to 16 against 13, with `act` naming the activity sparkline
// on one side and the detail button on the other — so a naive comparison would
// have called that a match and reported the wrong divergence.
//
// This one reads the shipped JavaScript and stays that way, decided rather than
// left over (#633). Every other rule that lived twice moved to a case list both
// suites read, because a rule is a behavior and a list can state it. This is not
// a rule: it is the question of whether the dashboard offers the same columns the
// TUI does, and the answer belongs to the TUI's own `columns` table. A fixture
// would be a third list for both sides to drift from, which is what the guard
// exists to catch.
//
// It is also not the scrape #633 retired: that one pulled a `const NAME = [...]`
// literal out of a file to compare it with a Go constant. This extracts the keys
// of a table of column definitions, which no constant declares anywhere.
func TestDashboardSharesTheColumnSet(t *testing.T) {
	b, err := os.ReadFile("../../internal/web/static/app.js")
	if err != nil {
		t.Fatalf("reading app.js: %v", err)
	}
	got := dashboardColumnKeys(t, string(b))
	want := columnKeys()
	have := map[string]bool{}
	for _, k := range got {
		have[k] = true
	}
	var missing []string
	for _, k := range want {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the dashboard has no column for %v — content parity is owed (#544)", missing)
	}

	known := map[string]bool{}
	for _, k := range want {
		known[k] = true
	}
	for _, k := range got {
		if !known[k] && !webExtraColumns[k] {
			t.Errorf("the dashboard has a column %q the TUI does not — either port it or declare it a gesture", k)
		}
	}
}

// keysFromColsBlock extracts the keys of the dashboard's column table, or nil
// when the table is not there.
//
// It reads `const COLS = [ … ];` and nothing else. Scanning the whole file
// instead meant any object of the same shape anywhere in it was reported as a
// column the TUI lacks — including, once, the comment that explained the
// previous collision (#692). Bounding the scan makes everything outside the
// table out of scope by construction rather than by luck of naming.
func keysFromColsBlock(src string) []string {
	const open = "const COLS = ["
	i := strings.Index(src, open)
	if i < 0 {
		return nil
	}
	block := src[i+len(open):]
	// The table closes on a line that is exactly `];`, at column zero — every
	// nested literal inside it is indented, so this cannot cut the block short.
	if j := strings.Index(block, "\n];"); j >= 0 {
		block = block[:j]
	} else {
		return nil // opened and never closed: the extraction cannot trust what follows
	}
	var got []string
	for _, line := range strings.Split(block, "\n") {
		const marker = "{ key: \""
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		rest := line[i+len(marker):]
		if j := strings.Index(rest, "\""); j >= 0 {
			got = append(got, rest[:j])
		}
	}
	return got
}

// dashboardColumnKeys is keysFromColsBlock with the verdict: an extraction that
// finds nothing has stopped working and must say so, not pass on an empty set.
func dashboardColumnKeys(t *testing.T, src string) []string {
	t.Helper()
	got := keysFromColsBlock(src)
	if len(got) == 0 {
		t.Fatal("no column table found in app.js — the extraction is broken, not the code")
	}
	return got
}

// #692. The extraction used to read the whole file, so any object of the same
// shape anywhere in app.js was reported as a dashboard column the TUI lacks.
//
// That is not hypothetical: #689 added a small table of stats periods in the
// ordinary shape and the guard failed with `the dashboard has a column "24h"`.
// Renaming its field cleared it — and the guard then matched the *comment*
// explaining the rename, because the comment spelled the pattern out.
//
// The failure names a defect that does not exist and prescribes a fix that is
// wrong ("either port it or declare it a gesture", about something that is not a
// column). The cost is the next person's: they add a keyed array, are told they
// broke column parity, and either contort their code or weaken this guard.
func TestTheColumnScrapeReadsOnlyTheColumnTable(t *testing.T) {
	src := `const SOMETHING_ELSE = [
  { key: "24h", days: 1 },
  { key: "7d", days: 7 },
];

// A comment that spells the pattern out: { key: "trap" } — prose is not code.

const COLS = [
  { key: "name", label: "Session" },
  { key: "status", label: "Status" },
];

const LATER = [
  { key: "decoy", label: "not a column" },
];
`
	got := dashboardColumnKeys(t, src)
	want := []string{"name", "status"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("extracted %v, want %v — anything outside the COLS table is not a column", got, want)
	}
}

// The no-match case must stay loud: an extraction that has stopped finding the
// table has to say so rather than pass on an empty set.
func TestTheColumnScrapeFailsLoudlyWithNoTable(t *testing.T) {
	if keysFromColsBlock("const NOTHING = [];\n") != nil {
		t.Error("found column keys in a source that declares no COLS table")
	}
}
