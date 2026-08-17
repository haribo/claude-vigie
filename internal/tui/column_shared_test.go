package tui

import (
	"encoding/json"
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
	b, err := os.ReadFile("../../test/fixtures/column-cases.json")
	if err != nil {
		t.Fatalf("reading the shared fixture: %v", err)
	}
	var f columnFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parsing the shared fixture: %v", err)
	}
	if len(f.Context) == 0 || len(f.Mode) == 0 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	return f
}

func TestContextCellAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadColumnFixture(t).Context {
		s := api.SessionView{Model: c.Model, ContextTokens: c.Tokens}
		if got := contextCell(s); got != c.Want {
			t.Errorf("contextCell(model=%q, tokens=%v) = %q, want %q — %s", c.Model, c.Tokens, got, c.Want, c.Why)
		}
	}
}

func TestModeLabelAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadColumnFixture(t).Mode {
		if got, _ := permissionModeLabel(c.Raw); got != c.Want {
			t.Errorf("permissionModeLabel(%q) = %q, want %q — %s", c.Raw, got, c.Want, c.Why)
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
func TestDashboardSharesTheColumnSet(t *testing.T) {
	b, err := os.ReadFile("../../internal/web/static/app.js")
	if err != nil {
		t.Fatalf("reading app.js: %v", err)
	}
	var got []string
	for _, line := range strings.Split(string(b), "\n") {
		const marker = `{ key: "`
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		rest := line[i+len(marker):]
		if j := strings.Index(rest, `"`); j >= 0 {
			got = append(got, rest[:j])
		}
	}
	if len(got) == 0 {
		t.Fatal("no `{ key: \"…\"` found in app.js — the extraction is broken, not the code")
	}

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
