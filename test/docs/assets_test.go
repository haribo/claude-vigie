package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The README animation is a hand-generated asset: nothing regenerates it, so
// nothing would notice it losing a property it was built to have. Each check
// below stands for something that actually went wrong while it was being made, or
// a decision that would be silently lost in a rebuild (#450).
//
// This is the half of #450 that is checkable today. It does not make the asset
// reproducible — that needs the generator, which is a separate decision.

var animationAssets = []string{
	"../../docs/assets/session-call.svg",
	"../../docs/assets/session-call-dark.svg",
	"../../site/assets/session-call.svg",
	"../../site/assets/session-call-dark.svg",
}

func readAsset(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // paths are our own committed assets
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// Without xml:space="preserve" the renderer collapses the runs of spaces that
// form the board's columns, and the table reads as one glued word. It happened.
func TestTheAnimationPreservesWhitespace(t *testing.T) {
	for _, p := range animationAssets {
		body := readAsset(t, p)
		texts := strings.Count(body, "<text")
		preserved := strings.Count(body, `xml:space="preserve"`)
		if preserved == 0 {
			t.Errorf("%s: no xml:space=\"preserve\" — the columns would collapse", filepath.Base(p))
			continue
		}
		if preserved != texts {
			t.Errorf("%s: %d <text> elements but %d preserve whitespace", filepath.Base(p), texts, preserved)
		}
	}
}

// Motion is opt-in: the asset shows its outcome with no animation at all, and
// only moves for a reader who has not asked for reduced motion.
func TestTheAnimationIsOptIn(t *testing.T) {
	for _, p := range animationAssets {
		body := readAsset(t, p)
		if !strings.Contains(body, "prefers-reduced-motion: no-preference") {
			t.Errorf("%s: animation is not gated behind prefers-reduced-motion", filepath.Base(p))
		}
		// Every animation rule must sit inside that gate: nothing may move by default.
		gate := strings.Index(body, "@media (prefers-reduced-motion: no-preference)")
		for _, m := range regexp.MustCompile(`animation:`).FindAllStringIndex(body, -1) {
			if m[0] < gate {
				t.Errorf("%s: an animation rule sits outside the reduced-motion gate", filepath.Base(p))
				break
			}
		}
	}
}

// The first version of this asset leaked three real project names. Whatever
// rebuilds it must not read them off a live machine.
func TestTheAnimationCarriesNoRealProjectNames(t *testing.T) {
	// The placeholders the asset is built from. Anything that looks like a project
	// path in the animation must be one of these.
	allowed := map[string]bool{"api-gateway": true, "web-app": true, "data-pipeline": true}
	name := regexp.MustCompile(`~/([a-z0-9][a-z0-9-]{2,})`)

	for _, p := range animationAssets {
		for _, m := range name.FindAllStringSubmatch(readAsset(t, p), -1) {
			if !allowed[m[1]] {
				t.Errorf("%s: %q is not one of the placeholder project names", filepath.Base(p), m[1])
			}
		}
	}
}

// The light and dark files are the same drawing in two palettes. If they drift
// apart structurally, one theme is showing something the other is not.
func TestTheThemesAreTheSameDrawing(t *testing.T) {
	color := regexp.MustCompile(`#[0-9a-fA-F]{6}`)
	theme := regexp.MustCompile(`-(light|dark)`)
	strip := func(s string) string {
		return theme.ReplaceAllString(color.ReplaceAllString(s, "COLOR"), "-THEME")
	}

	for _, pair := range [][2]string{
		{animationAssets[0], animationAssets[1]},
		{animationAssets[2], animationAssets[3]},
	} {
		if strip(readAsset(t, pair[0])) != strip(readAsset(t, pair[1])) {
			t.Errorf("%s and %s differ by more than their palette",
				filepath.Base(filepath.Dir(pair[0]))+"/"+filepath.Base(pair[0]), filepath.Base(pair[1]))
		}
	}
}

// The README and the site ship the same animation. Updating one and forgetting
// the other is the obvious way for them to disagree.
func TestTheReadmeAndSiteCopiesAgree(t *testing.T) {
	for i := 0; i < 2; i++ {
		docs, site := readAsset(t, animationAssets[i]), readAsset(t, animationAssets[i+2])
		if docs != site {
			t.Errorf("%s differs between docs/assets and site/assets", filepath.Base(animationAssets[i]))
		}
	}
}

// The README must offer both themes, or a dark-theme reader gets the light asset.
func TestTheReadmeOffersBothThemes(t *testing.T) {
	readme := readAsset(t, "../../README.md")
	for _, want := range []string{
		"docs/assets/session-call.svg",
		"docs/assets/session-call-dark.svg",
		"prefers-color-scheme: dark",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README does not reference %q", want)
		}
	}
}
