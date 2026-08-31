package docs_test

import (
	"regexp"
	"strings"
	"testing"
)

// What the README promises about platforms is decided by `.goreleaser.yaml`, and
// the two had drifted apart: the README sold "cross-platform" in a design bullet
// while every published archive was `linux/*` and session presence read `/proc`
// (#672). A reader decides on that sentence before cloning, so someone on macOS
// installed and found that the machinery answering "which session needs me" did
// not work.
//
// The other doc tests check that documents have a status and a shape. This one
// checks a claim, against the file that settles it — the prose is where both
// halves of #672 lived, and nothing read the prose.

var goosRe = regexp.MustCompile(`goos:\s*\[([^\]]*)\]`)

// releaseTargets returns the distinct GOOS values every build block declares.
func releaseTargets(t *testing.T) []string {
	t.Helper()
	src := read(t, "../../.goreleaser.yaml")
	seen := map[string]bool{}
	var out []string
	for _, m := range goosRe.FindAllStringSubmatch(src, -1) {
		for _, os := range strings.Split(m[1], ",") {
			os = strings.TrimSpace(os)
			if os != "" && !seen[os] {
				seen[os] = true
				out = append(out, os)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no goos declared in .goreleaser.yaml — this test can no longer tell what ships")
	}
	return out
}

func TestTheReadmeDoesNotPromiseAPlatformTheReleaseDoesNotBuild(t *testing.T) {
	targets := releaseTargets(t)
	if len(targets) > 1 || targets[0] != "linux" {
		t.Skipf("the release builds %v; this check is about the single-platform case", targets)
	}

	readme := read(t, "../../README.md")
	if strings.Contains(strings.ToLower(readme), "cross-platform") {
		t.Error("the README calls vigie cross-platform while the release builds linux only")
	}
	if !strings.Contains(readme, "**Linux only.**") {
		t.Error("the README does not state the platform requirement; a reader decides on it before cloning")
	}
}
