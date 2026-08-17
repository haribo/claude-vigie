package animation

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The asset used to be unreproducible: generated once by a script nobody
// committed, so a change to the TUI's rendering would leave it showing a product
// that no longer exists and only a human noticing would catch it (#450).
//
// These tests are what closes that. The committed files must equal a fresh
// render, so editing the template without running `just docs-animation` fails the
// build, and editing a committed file by hand fails it too.

const repoRoot = "../.."

func TestTheCommittedAssetsAreWhatTheGeneratorProduces(t *testing.T) {
	for _, p := range Themes() {
		svg, err := Render(p)
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		for _, target := range Targets(p) {
			committed, err := os.ReadFile(filepath.Join(repoRoot, target)) //nolint:gosec // our own committed asset
			if err != nil {
				t.Fatalf("reading %s: %v", target, err)
			}
			if string(committed) != svg {
				t.Errorf("%s is not what the generator produces — run `just docs-animation`", target)
			}
		}
	}
}

// Every target is a real path. A typo here would silently write the asset
// somewhere nobody looks.
func TestEveryTargetExists(t *testing.T) {
	for _, p := range Themes() {
		targets := Targets(p)
		if len(targets) != 2 {
			t.Errorf("%s: %d targets, want the README copy and the site copy", p.Name, len(targets))
		}
		for _, target := range targets {
			if _, err := os.Stat(filepath.Join(repoRoot, target)); err != nil {
				t.Errorf("%s: %v", target, err)
			}
		}
	}
}

// The palettes must be complete: an empty color renders as an invalid attribute
// and paints nothing, which is easy to miss on a dark background.
func TestBothPalettesAreComplete(t *testing.T) {
	hex := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for _, p := range Themes() {
		for name, value := range map[string]string{
			"Accent": p.Accent, "Background": p.Background, "Panel": p.Panel,
			"Border": p.Border, "Chrome": p.Chrome, "Dim": p.Dim,
			"Text": p.Text, "Green": p.Green, "Amber": p.Amber, "Blue": p.Blue,
		} {
			if !hex.MatchString(value) {
				t.Errorf("%s palette: %s = %q, want a #rrggbb color", p.Name, name, value)
			}
		}
		if p.Name == "" {
			t.Error("a palette has no name — the clip-path ids would collide between themes")
		}
	}
}

// The two themes must not share a color: identical palettes would mean one theme
// is unreadable, and the template alone cannot catch that.
func TestTheThemesActuallyDiffer(t *testing.T) {
	light, dark := lightPalette, darkPalette
	if light.Background == dark.Background || light.Text == dark.Text {
		t.Error("the light and dark palettes share their background or text color")
	}
	if light.Name == dark.Name {
		t.Error("both palettes carry the same name")
	}
}

// A rendered asset must keep the properties the drawing depends on. These are
// also checked on the committed files in test/docs; here they guard the template
// itself, so a bad edit fails before anything is written.
func TestARenderedAssetKeepsItsProperties(t *testing.T) {
	for _, p := range Themes() {
		svg, err := Render(p)
		if err != nil {
			t.Fatal(err)
		}
		if texts, preserved := strings.Count(svg, "<text"), strings.Count(svg, `xml:space="preserve"`); texts != preserved {
			t.Errorf("%s: %d <text> but %d preserve whitespace — the columns would collapse", p.Name, texts, preserved)
		}
		if !strings.Contains(svg, "prefers-reduced-motion: no-preference") {
			t.Errorf("%s: motion is not opt-in", p.Name)
		}
		if strings.Contains(svg, "{{") {
			t.Errorf("%s: an unrendered template action survived into the output", p.Name)
		}
	}
}

// The clip-path ids carry the theme name so the two files can sit on one page
// without colliding — the README serves both through <picture>.
func TestClipPathIdsAreThemeScoped(t *testing.T) {
	for _, p := range Themes() {
		svg, err := Render(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(svg, `id="c1-`+p.Name+`"`) {
			t.Errorf("%s: clip-path ids are not scoped to the theme", p.Name)
		}
		other := "dark"
		if p.Name == "dark" {
			other = "light"
		}
		if strings.Contains(svg, "-"+other+`"`) {
			t.Errorf("%s: carries the other theme's ids", p.Name)
		}
	}
}

// #571. The hero is drawn from a template for the same reason the animation is:
// it could not be rebuilt, so nothing could check it, and it drifted — its two
// dark copies had ended up in different palettes, and neither matched the other.
//
// These are the twins of the guards above. Without them the hero would be
// regenerable in principle and unchecked in practice, which is the state that
// produced the drift.
func TestTheCommittedHeroesAreWhatTheGeneratorProduces(t *testing.T) {
	for _, p := range HeroThemes() {
		svg, err := RenderHero(p)
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		for _, target := range HeroTargets(p) {
			committed, err := os.ReadFile(filepath.Join(repoRoot, target)) //nolint:gosec // our own committed asset
			if err != nil {
				t.Fatalf("reading %s: %v", target, err)
			}
			if string(committed) != svg {
				t.Errorf("%s is not what the generator produces — run `just docs-animation`", target)
			}
		}
	}
}

// The two copies are one drawing. They were maintained by hand and diverged;
// rendering both from one template is what stops that, and this says so.
func TestBothHeroCopiesAreIdentical(t *testing.T) {
	for _, p := range HeroThemes() {
		targets := HeroTargets(p)
		if len(targets) != 2 {
			t.Fatalf("%s: %d targets, want the README copy and the site copy", p.Name, len(targets))
		}
		a, err := os.ReadFile(filepath.Join(repoRoot, targets[0])) //nolint:gosec // our own committed asset
		if err != nil {
			t.Fatalf("reading %s: %v", targets[0], err)
		}
		b, err := os.ReadFile(filepath.Join(repoRoot, targets[1])) //nolint:gosec // our own committed asset
		if err != nil {
			t.Fatalf("reading %s: %v", targets[1], err)
		}
		if string(a) != string(b) {
			t.Errorf("%s and %s are not the same drawing", targets[0], targets[1])
		}
	}
}

// An empty color renders as an invalid attribute and paints nothing, which is
// easy to miss on a dark background.
func TestBothHeroPalettesAreComplete(t *testing.T) {
	for _, p := range HeroThemes() {
		v := reflect.ValueOf(p)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).String() == "" {
				t.Errorf("%s palette: %s is empty", p.Name, v.Type().Field(i).Name)
			}
		}
	}
}

// The window buttons are the same three colors in both themes on purpose — they
// are macOS chrome, not part of the palette. Everything else must differ, or a
// theme is not a theme.
func TestTheHeroThemesActuallyDiffer(t *testing.T) {
	light, dark := heroLight, heroDark
	lv, dv := reflect.ValueOf(light), reflect.ValueOf(dark)
	for i := 0; i < lv.NumField(); i++ {
		name := lv.Type().Field(i).Name
		if name == "Name" {
			continue
		}
		if lv.Field(i).String() == dv.Field(i).String() {
			t.Errorf("%s is %s in both themes", name, lv.Field(i).String())
		}
	}
}
