package docs_test

import (
	"regexp"
	"strings"
	"testing"
)

// `pr-issue-check.yaml` decides whether a pull request says which issue it
// belongs to. It is a required check, so what it accepts is the contract every
// contributor writes against — and it disagreed with the repository's own
// documentation.
//
// `docs/git-workflow.md` and `.claude/commands/gh-merge-develop.md` both describe
// `Part of #N`: a PR that advances an issue without finishing it, which closes
// nothing on merge. The check accepted only the closing keywords, so such a PR
// failed with "PR must reference a GitHub issue in the body" — about a body that
// referenced one (#703).
//
// The rule is scraped from the workflow rather than restated here. A copy would
// be a second thing to keep true, which is the class of defect #671 was about.

// acceptRe pulls the grep pattern the workflow accepts a body on.
func acceptRe(t *testing.T) *regexp.Regexp {
	t.Helper()
	src := read(t, "../../.github/workflows/pr-issue-check.yaml")
	m := regexp.MustCompile(`grep -qiE '([^']+)'\s*;\s*then\s*\n\s*echo "Issue reference found`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("could not find the body pattern in pr-issue-check.yaml — the extraction is broken, not the workflow")
	}
	re, err := regexp.Compile("(?i)" + m[1])
	if err != nil {
		t.Fatalf("the workflow's pattern does not compile in Go: %v", err)
	}
	return re
}

func TestTheIssueCheckAcceptsEveryFormTheDocsDescribe(t *testing.T) {
	re := acceptRe(t)

	accepted := map[string]string{
		"Closes #12":   "the closing form, the common case",
		"closes #12":   "case does not matter, the check greps case-insensitively",
		"Fixes #7":     "an alternative closing keyword",
		"Resolves #99": "and the third",
		"Part of #661": "a PR that advances an issue without finishing it — docs/git-workflow.md",
		"part of #661": "same, lowercased",
	}
	for body, why := range accepted {
		if !re.MatchString(body) {
			t.Errorf("the check refuses %q — %s", body, why)
		}
	}

	refused := map[string]string{
		"":                          "a body with no issue at all is what the check is for",
		"see the issue":             "prose naming no number references nothing",
		"related to the tracker":    "neither a keyword nor a number",
		"this one is obvious to me": "the check exists so that is not a reason",
	}
	for body, why := range refused {
		if re.MatchString(body) {
			t.Errorf("the check accepts %q — %s", body, why)
		}
	}
}

// The exemption is separate from the reference and must stay narrow: a `chore` or
// `style` PR skips the requirement entirely, and nothing else does.
func TestOnlyChoreAndStyleSkipTheRequirement(t *testing.T) {
	src := read(t, "../../.github/workflows/pr-issue-check.yaml")
	m := regexp.MustCompile(`grep -qiE '(\^\([^']+)'`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("could not find the title pattern in pr-issue-check.yaml")
	}
	re := regexp.MustCompile("(?i)" + m[1])

	for _, title := range []string{"chore: bump", "style: reflow"} {
		if !re.MatchString(title) {
			t.Errorf("%q should be exempt", title)
		}
	}
	for _, title := range []string{"fix(watch): a real change", "feat(web): a real change", "docs: a real change"} {
		if re.MatchString(title) {
			t.Errorf("%q must not be exempt — it is the kind of change an issue exists for", strings.SplitN(title, ":", 2)[0])
		}
	}
}
