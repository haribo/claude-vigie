package docs_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/status"
)

// #718. `stalled` was removed from the vocabulary and the word stayed behind: the
// README still advertised nine live statuses, the specification's own section was
// still titled "The nine statuses" over a table of eight, and the site and the
// GNOME settings still offered to tell the operator about a status that no longer
// exists.
//
// The existing guard did not catch any of it. It scrapes the specification's
// *table rows* and compares them with the code, so it passes while the heading
// above the table and the paragraph below it contradict both. Green on a document
// disagreeing with itself is what this project says about line coverage, arriving
// from the other side.
//
// These two checks cover what that one cannot: the number, and the words.

// numberWords are the spellings a status count could take. The count is small and
// will stay small; a numeral is not accepted on purpose, because the prose spells
// it out and that is the form that drifted.
var numberWords = []string{
	"zero", "one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "ten", "eleven", "twelve",
}

// Two counts appear in the prose, and both are derivable from the code — so both
// are checked against it rather than against each other. A count of some other
// subset ("an allow list naming three statuses") is history and is left alone.
var counts = []struct {
	what string
	re   *regexp.Regexp
	want func() int
}{
	{"the whole vocabulary", regexp.MustCompile(`(?i)\b([a-z]+) live statuses\b`), func() int { return len(status.All) }},
	{"the whole vocabulary", regexp.MustCompile(`(?im)^## 1\. The ([a-z]+) statuses`), func() int { return len(status.All) }},
	{"the attention set", regexp.MustCompile(`(?i)\b([a-z]+) statuses that call the operator\b`), func() int { return len(status.Attention) }},
}

func TestTheProseCountsTheStatusesCorrectly(t *testing.T) {
	for _, path := range []string{"../../README.md", "../../docs/design/session-status.md"} {
		src := read(t, path)
		for _, c := range counts {
			for _, m := range c.re.FindAllStringSubmatch(src, -1) {
				want := numberWords[c.want()]
				if got := strings.ToLower(m[1]); got != want {
					t.Errorf("%s says %q for %s; there are %d, which is %q",
						shortPath(path), strings.TrimSpace(m[0]), c.what, c.want(), want)
				}
			}
		}
	}
}

// The surfaces a user meets before reading any code. They carry no history, so a
// status named there is a claim about what the product does today — unlike the
// specification and the ADRs, which record what was removed and why.
var userFacing = []string{
	"../../README.md",
	"../../site/index.html",
	"../../gnome-extension/schemas/org.gnome.shell.extensions.claude-vigie.gschema.xml",
	"../../gnome-extension/README.md",
}

// retired are statuses this project has shipped and withdrawn. Naming one outside
// its own record is the drift #718 was about.
var retired = []string{"stalled"}

func TestNoUserFacingTextNamesARetiredStatus(t *testing.T) {
	for _, path := range userFacing {
		src := strings.ToLower(read(t, path))
		for _, s := range retired {
			if regexp.MustCompile(`\b` + s + `\b`).MatchString(src) {
				t.Errorf("%s names %q, which is not a status vigie has (see ADR-0012)", shortPath(path), s)
			}
		}
		// And the live vocabulary must not have gained a member the docs missed:
		// every status the code has should be nameable, so a status absent from all
		// four surfaces is not an error — but one named that does not exist is.
	}
}

func shortPath(p string) string { return strings.TrimPrefix(p, "../../") }
