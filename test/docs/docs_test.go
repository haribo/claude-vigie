// Package docs_test checks the shape of the documents the project treats as its
// source of truth. It asserts nothing about their content — only that a reader
// can tell what a document is and whether it describes shipped behavior.
//
// It exists because tui-preflight.md went a whole release cycle with no status
// line at all, and five specifications stayed marked "Proposed" after the work
// they specify had shipped (#437). Whether a status is *correct* needs the issue
// tracker and cannot be checked here; that it *exists* can.
package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const designDir = "../../docs/design"
const adrDir = "../../docs/adr"

func markdownIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	if len(out) == 0 {
		t.Fatalf("no markdown found in %s — has the layout changed?", dir)
	}
	return out
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // paths come from our own docs directory
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

var statusLine = regexp.MustCompile(`(?m)^\*\*Status:\*\* (Proposed|Accepted|Superseded|Deprecated)\b`)

// TestEverySpecificationDeclaresItsStatus: a reader must be able to tell a
// specification of shipped behavior from one of an intention.
func TestEverySpecificationDeclaresItsStatus(t *testing.T) {
	for _, p := range markdownIn(t, designDir) {
		if !statusLine.MatchString(read(t, p)) {
			t.Errorf("%s has no `**Status:** …` line", filepath.Base(p))
		}
	}
}

// TestEverySpecificationIsTitledAsOne: the convention makes a specification
// recognizable from its first line. tui-preflight.md was the one that drifted.
func TestEverySpecificationIsTitledAsOne(t *testing.T) {
	for _, p := range markdownIn(t, designDir) {
		first, _, _ := strings.Cut(read(t, p), "\n")
		if !strings.HasPrefix(first, "# ") {
			t.Errorf("%s does not open with a level-1 heading: %q", filepath.Base(p), first)
			continue
		}
		if !strings.HasSuffix(first, "— Design Specification") {
			t.Errorf("%s is titled %q, want it to end with \"— Design Specification\"",
				filepath.Base(p), first)
		}
	}
}

// TestEveryADRDeclaresItsStatus mirrors the rule for decisions. An ADR is never
// deleted, so its status is the only thing saying whether it still applies.
func TestEveryADRDeclaresItsStatus(t *testing.T) {
	heading := regexp.MustCompile(`(?m)^## Status\s*\n+\s*(Accepted|Superseded|Deprecated|Proposed)\b`)
	for _, p := range markdownIn(t, adrDir) {
		if !heading.MatchString(read(t, p)) {
			t.Errorf("%s has no `## Status` section naming a status", filepath.Base(p))
		}
	}
}

// TestSupersededADRsLinkBothWays: a one-sided link is how the chain rots. The
// project rule is that a reversal carries `Superseded by` on the old ADR and
// `Supersedes` on the new one.
func TestSupersededADRsLinkBothWays(t *testing.T) {
	link := regexp.MustCompile(`\[ADR-(\d{4})\]`)
	for _, p := range markdownIn(t, adrDir) {
		body := read(t, p)
		lower := strings.ToLower(body)
		if !strings.Contains(lower, "superseded by") {
			continue
		}
		// The successor named here must point back at this ADR.
		self := strings.SplitN(filepath.Base(p), "-", 2)[0]
		var linksBack bool
		for _, m := range link.FindAllStringSubmatch(body, -1) {
			for _, q := range markdownIn(t, adrDir) {
				if !strings.HasPrefix(filepath.Base(q), m[1]) {
					continue
				}
				if strings.Contains(read(t, q), "ADR-"+self) {
					linksBack = true
				}
			}
		}
		if !linksBack {
			t.Errorf("%s says it is superseded but no ADR links back to it", filepath.Base(p))
		}
	}
}

// The rename to claude-vigie (#262/#263) left `fleetd` in the deployment guide,
// where an operator follows commands verbatim — seven times, in the one document
// whose instructions expose a server if they are wrong (#467).
//
// Nothing detected it: the shape checks above look at how a document is
// structured, never at whether the prose names something that exists.
func TestNoDocumentNamesTheOldBinary(t *testing.T) {
	gone := regexp.MustCompile(`\bfleetd\b`)
	for _, dir := range []string{"../../docs", "../../docs/design", "../../docs/adr"} {
		for _, p := range markdownIn(t, dir) {
			if gone.MatchString(read(t, p)) {
				t.Errorf("%s names `fleetd`, a binary that does not exist — it is `vigied`", filepath.Base(p))
			}
		}
	}
	for _, p := range []string{"../../README.md", "../../gnome-extension/README.md"} {
		if gone.MatchString(read(t, p)) {
			t.Errorf("%s names `fleetd`", p)
		}
	}
}

// docs/code.md § File organization lists every package a contributor is meant to
// find. It said the web dashboard was "**planned**, not yet present" long after it
// shipped, and had missed a dozen packages added since (#468).
//
// The check is symmetric on purpose: a package with no row is undiscoverable, and
// a row with no package sends someone looking for something that is not there.
func TestThePackageTableMatchesTheTree(t *testing.T) {
	row := regexp.MustCompile("(?m)^\\| `internal/([a-z]+)` \\|")
	listed := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(read(t, "../../docs/code.md"), -1) {
		listed[m[1]] = true
	}
	entries, err := os.ReadDir("../../internal")
	if err != nil {
		t.Fatalf("reading internal/: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && !listed[e.Name()] {
			t.Errorf("internal/%s has no row in docs/code.md — a contributor cannot find it", e.Name())
		}
		delete(listed, e.Name())
	}
	for name := range listed {
		t.Errorf("docs/code.md lists internal/%s, which does not exist", name)
	}
}
