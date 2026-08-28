package status

import (
	"encoding/json"
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
	designDoc = "../../docs/design/session-status.md"
	listDoc   = "../../docs/design/session-list.md"
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
// The clients keep an ordered copy of the vocabulary for presentation, and both
// read this list under node. A Go test used to pull each literal out of the
// shipped JavaScript with a regular expression instead, which is what #633
// retired: a scrape can check that a constant matches and nothing about what the
// client does with it, and it could only reach a constant that was module-private
// by not importing it.
func TestTheStatusVocabularyMatchesTheSharedFixture(t *testing.T) {
	var f struct {
		Order []string `json:"order"`
	}
	b, err := os.ReadFile("../../test/fixtures/status-vocabulary.json")
	if err != nil {
		t.Fatalf("reading the shared fixture: %v", err)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parsing the shared fixture: %v", err)
	}
	if len(f.Order) == 0 {
		t.Fatal("the shared fixture has no statuses — the extraction is broken, not the code")
	}
	diff(t, "test/fixtures/status-vocabulary.json", f.Order, All)
}

func TestAllMatchesTheDesignDocument(t *testing.T) {
	diff(t, designDoc+" § 1", statusesFromDoc(t), All)
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

// TestAttentionIsASubsetOfAll: an attention status that does not exist would
// silently never fire.
func TestAttentionIsASubsetOfAll(t *testing.T) {
	for _, s := range Attention {
		if !Known(s) {
			t.Errorf("Attention names %q, which is not a status", s)
		}
		if !NeedsAttention(s) {
			t.Errorf("NeedsAttention(%q) = false", s)
		}
	}
	for _, s := range []string{"working", "idle", "ended", "thinking", "compacting", "stale", ""} {
		if NeedsAttention(s) {
			t.Errorf("NeedsAttention(%q) = true — it does not block the operator", s)
		}
	}
}

// docSection isolates a numbered section of the design document, the way
// sectionOne does for § 1. It exists so a claim in one section is never read as
// if it belonged to another.
func docSection(doc, heading string) string {
	start := strings.Index(doc, heading)
	if start < 0 {
		return ""
	}
	rest := doc[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// TestEveryStatusHasADetectionRow guards § 5, which answers "what produces this
// status, and how much should I trust it" — one row per status.
//
// `stale` had no row. The one status whose entire meaning is *nothing is
// observing this machine* was missing from the table about who observes what,
// and § 3 two sections above still claimed a silent session simply reads `ended`
// — the behavior #284/#285 replaced (#539).
//
// Nothing would have caught it. #423 pinned § 1's vocabulary against the code and
// the two JavaScript copies; the table that says where each of those statuses
// comes from was checked by nobody. A status added to § 1 and never given a row
// is one the reader is invited to trust with no stated basis.
func TestEveryStatusHasADetectionRow(t *testing.T) {
	b, err := os.ReadFile(designDoc)
	if err != nil {
		t.Fatalf("reading the design document: %v", err)
	}
	five := docSection(string(b), "## 5.")
	if five == "" {
		t.Fatalf("%s has no `## 5.` section — this guard needs updating", designDoc)
	}
	// Each row opens with the status in backticks, as in § 1.
	row := regexp.MustCompile("(?m)^\\|\\s*`([a-z]+)`\\s*\\|")
	rows := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(five, -1) {
		rows[m[1]] = true
	}
	if len(rows) == 0 {
		t.Fatalf("no status rows parsed out of %s § 5 — has the table moved?", designDoc)
	}
	var missing []string
	for _, s := range All {
		if !rows[s] {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		t.Errorf("§ 5 has no detection row for %v — each status must say what produces it", missing)
	}
	for s := range rows {
		if !Known(s) {
			t.Errorf("§ 5 has a row for %q, which is not a status", s)
		}
	}
}
