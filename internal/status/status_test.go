package status

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The vocabulary is copied into consumers that cannot import Go — the web
// dashboard and the GNOME extension. Copies are allowed; silent divergence is
// not. These tests are the check: they read the design document and each copy,
// and report exactly which entries drift.
//
// This is what #423 buys. Before it, adding `compacting` (#342) reached two of
// four consumers: the metrics gauge dropped those sessions from its series
// (#421) and the GNOME extension dropped them from its menu entirely (#422).

const (
	designDoc    = "../../docs/design/session-status.md"
	listDoc      = "../../docs/design/session-list.md"
	dashboardJS  = "../../internal/web/static/app.js"
	dashboardLib = "../../internal/web/static/lib.js"
	gnomeJS      = "../../gnome-extension/extension.js"
)

// statusesFromDoc reads § 1's table, which is where the vocabulary is specified.
// Each row opens with the status in backticks: "| `working`  | Claude is …".
func statusesFromDoc(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(designDoc)
	if err != nil {
		t.Fatalf("reading the design document: %v", err)
	}
	row := regexp.MustCompile("(?m)^\\|\\s*`([a-z]+)`\\s*\\|")
	var out []string
	for _, line := range strings.Split(sectionOne(string(b)), "\n") {
		if m := row.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		t.Fatalf("no status rows found in %s § 1 — has the table moved?", designDoc)
	}
	return out
}

// sectionOne isolates § 1 so a status named in a later section (the sort order,
// the reliability table) cannot be mistaken for a declaration.
func sectionOne(doc string) string {
	start := strings.Index(doc, "## 1.")
	if start < 0 {
		return ""
	}
	rest := doc[start+len("## 1."):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// jsArray extracts a `const NAME = ["a", "b"]` literal from a JavaScript file.
func jsArray(t *testing.T, path, name string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	decl := regexp.MustCompile(`const\s+` + name + `\s*=\s*\[([^\]]*)\]`)
	m := decl.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("%s: no `const %s = [...]` found — was it renamed?", path, name)
	}
	var out []string
	for _, raw := range strings.Split(m[1], ",") {
		if v := strings.Trim(strings.TrimSpace(raw), `"'`); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func diff(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") == strings.Join(want, ",") {
		return
	}
	inWant := map[string]bool{}
	for _, s := range want {
		inWant[s] = true
	}
	inGot := map[string]bool{}
	for _, s := range got {
		inGot[s] = true
	}
	var missing, extra []string
	for _, s := range want {
		if !inGot[s] {
			missing = append(missing, s)
		}
	}
	for _, s := range got {
		if !inWant[s] {
			extra = append(extra, s)
		}
	}
	switch {
	case len(missing) > 0 || len(extra) > 0:
		t.Errorf("%s drifted: missing %v, unexpected %v\n  got  %v\n  want %v",
			what, missing, extra, got, want)
	default:
		t.Errorf("%s has the right statuses in the wrong order:\n  got  %v\n  want %v", what, got, want)
	}
}

// sorted copies and sorts, so a set comparison does not depend on order.
func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// statusesFromSortTable reads the ranked table of docs/design/session-list.md
// § 2.1, whose rows open with the rank then the status: "| 1 | `stalled` | …".
func statusesFromSortTable(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(listDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", listDoc, err)
	}
	row := regexp.MustCompile("(?m)^\\|\\s*(\\d+)\\s*\\|\\s*`([a-z]+)`")
	rows := row.FindAllStringSubmatch(string(b), -1)
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		out = append(out, m[2])
	}
	if len(out) == 0 {
		t.Fatalf("no ranked rows found in %s § 2.1 — has the table moved?", listDoc)
	}
	return out
}

// TestAllMatchesTheDesignDocument keeps the code honest against the
// specification, in that direction: the document decides, the code follows.
func TestAllMatchesTheDesignDocument(t *testing.T) {
	diff(t, designDoc+" § 1", statusesFromDoc(t), All)
}

// TestDashboardKnowsEveryStatus: app.js uses STATUSES to decide whether a status
// can be styled, falling back to "idle". A status missing here is displayed as
// something it is not.
func TestDashboardKnowsEveryStatus(t *testing.T) {
	diff(t, dashboardJS+" STATUSES", jsArray(t, dashboardJS, "STATUSES"), All)
}

// TestGnomeExtensionKnowsEveryStatus: the extension groups its menu by this list.
// A status missing here is a session the operator never sees — which is how a
// `stalled` session, the one most worth a look, went invisible (#422).
func TestGnomeExtensionKnowsEveryStatus(t *testing.T) {
	diff(t, gnomeJS+" STATUS_ORDER", jsArray(t, gnomeJS, "STATUS_ORDER"), All)
}

func TestKnown(t *testing.T) {
	for _, s := range All {
		if !Known(s) {
			t.Errorf("Known(%q) = false", s)
		}
	}
	for _, s := range []string{"", "compact", "Working", "call"} {
		if Known(s) {
			t.Errorf("Known(%q) = true", s)
		}
	}
}

// TestOrderCoversEveryStatus is the guard #464 was missing: membership and
// ranking are separate lists, and a status can exist while nothing ranks it.
func TestOrderCoversEveryStatus(t *testing.T) {
	diff(t, "internal/status.Order", sorted(Order), sorted(All))
}

// TestOrderMatchesTheDesignDocument: § 2.1 decides the order, the code follows.
func TestOrderMatchesTheDesignDocument(t *testing.T) {
	diff(t, listDoc+" § 2.1", statusesFromSortTable(t), Order)
}

// TestRankPutsTheUnknownLast: a status this build does not know sorts last, never
// first — the opposite of the TUI's old default, which ranked four real statuses
// below `ended`.
func TestRankPutsTheUnknownLast(t *testing.T) {
	if Rank("quantum") <= Rank("ended") {
		t.Errorf("Rank(unknown) = %d, Rank(ended) = %d — an unknown status must sort last", Rank("quantum"), Rank("ended"))
	}
	for i, s := range Order {
		if Rank(s) != i {
			t.Errorf("Rank(%q) = %d, want %d", s, Rank(s), i)
		}
	}
}

// TestDashboardRanksEveryStatus: the web comparator subtracts two ranks, so a
// status missing from the list yielded undefined and therefore NaN — a comparator
// that returns NaN does not order at all, and the table came out with `ended`
// first (#464).
func TestDashboardRanksEveryStatus(t *testing.T) {
	diff(t, dashboardLib+" RANK_ORDER", jsArray(t, dashboardLib, "RANK_ORDER"), Order)
}
