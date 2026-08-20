package docs_test

import (
	"regexp"
	"strings"
	"testing"
)

// The changelog rules live in docs/changelog.md. This guards the one that
// actually eroded.
//
// `git-workflow.md` has always said a PR "adds **a line**" under `[Unreleased]`.
// By 0.6.0 the median entry was 138 words and the longest 218 — paragraphs
// explaining mechanisms, rejected alternatives and failure modes, all of which
// already existed in the issue and the PR the entry references. Keep a Changelog
// puts it as "the headline and the hook, not the full story"; the story had won.
//
// A rule nobody can fail is a rule that erodes, so this counts.

// maxEntryWords is the ceiling. Two full sentences plus an issue reference fit
// comfortably under it, so this fires on essays and never on prose.
const maxEntryWords = 60

// unreleasedEntries returns the `- ` bullets under `## [Unreleased]`, each
// flattened to one string.
//
// Only `[Unreleased]`, on purpose: that is where entries are written, and it is
// the moment the rule can still be applied for free. The released sections were
// brought into line once, deliberately, when the rule was adopted — 0.4.1, 0.5.0
// and 0.6.0 rewritten, 0.1.0 to 0.4.0 left alone because they were already short
// and carry no issue references, so their text is the only record there is
// (docs/changelog.md).
func unreleasedEntries(t *testing.T) []string {
	t.Helper()
	body := read(t, "../../CHANGELOG.md")
	start := strings.Index(body, "## [Unreleased]")
	if start < 0 {
		t.Fatal("CHANGELOG.md has no `## [Unreleased]` section — this guard needs updating")
	}
	rest := body[start+len("## [Unreleased]"):]
	if end := strings.Index(rest, "\n## ["); end >= 0 {
		rest = rest[:end]
	}

	var entries []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			entries = append(entries, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, line := range strings.Split(rest, "\n") {
		switch {
		case strings.HasPrefix(line, "- "):
			flush()
			cur = make([]string, 0, 4)
			cur = append(cur, strings.TrimPrefix(line, "- "))
		case strings.HasPrefix(line, "  ") && len(cur) > 0:
			cur = append(cur, strings.TrimSpace(line))
		case strings.HasPrefix(line, "###"), strings.TrimSpace(line) == "":
			// A blank line inside an entry is rare and does not end it; a heading does.
			if strings.HasPrefix(line, "###") {
				flush()
			}
		default:
			flush()
		}
	}
	flush()
	return entries
}

// firstWords is what an error message shows, so the failing entry is
// identifiable without opening the file.
func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) <= n {
		return s
	}
	return strings.Join(f[:n], " ") + "…"
}

func TestUnreleasedEntriesAreOneOrTwoSentences(t *testing.T) {
	for _, e := range unreleasedEntries(t) {
		if n := len(strings.Fields(e)); n > maxEntryWords {
			t.Errorf("a changelog entry is %d words, the ceiling is %d — the issue it references "+
				"is where the reasoning belongs (docs/changelog.md):\n  %s",
				n, maxEntryWords, firstWords(e, 12))
		}
	}
}

// Brevity is only safe because the entry points somewhere. An entry with no
// issue reference has to carry its own context, and then the ceiling is the
// wrong rule for it.
func TestUnreleasedEntriesReferenceAnIssue(t *testing.T) {
	ref := regexp.MustCompile(`#\d+`)
	for _, e := range unreleasedEntries(t) {
		if !ref.MatchString(e) {
			t.Errorf("a changelog entry references no issue — that reference is what makes "+
				"a one-line entry enough (docs/changelog.md):\n  %s", firstWords(e, 12))
		}
	}
}

// Keep a Changelog fixes the order, and a reader scanning several versions
// should not have to re-find the categories in each one.
func TestUnreleasedCategoriesAreInTheCanonicalOrder(t *testing.T) {
	canonical := []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}
	rank := map[string]int{}
	for i, c := range canonical {
		rank[c] = i
	}

	body := read(t, "../../CHANGELOG.md")
	start := strings.Index(body, "## [Unreleased]")
	rest := body[start:]
	if end := strings.Index(rest[len("## [Unreleased]"):], "\n## ["); end >= 0 {
		rest = rest[:end+len("## [Unreleased]")]
	}

	last, lastName := -1, ""
	for _, line := range strings.Split(rest, "\n") {
		name, ok := strings.CutPrefix(line, "### ")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		r, known := rank[name]
		if !known {
			t.Errorf("`### %s` is not one of Keep a Changelog's categories: %v", name, canonical)
			continue
		}
		if r < last {
			t.Errorf("`### %s` comes after `### %s`; the order is %v", name, lastName, canonical)
		}
		last, lastName = r, name
	}
}
