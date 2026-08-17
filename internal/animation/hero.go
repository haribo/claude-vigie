package animation

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

// The hero shot — the README's lead image and the landing page's — drawn from a
// template for the same reason the animation is (#450): an asset nobody can
// rebuild is one nothing can check, and this one had drifted. It showed a bottom
// bar carrying `synced` and `platform ● operational`, both moved into the state
// modal by #494, and its two dark copies had diverged from each other into
// separate palettes (#571).
//
// It has its own palette type rather than reusing the animation's. The drawing
// needs two roles the animation does not — a frame stroke distinct from the inner
// borders, and a selection band — and its panel and background differ. Forcing it
// onto the shared palette would have changed how the image looks, which is a
// judgement no test can make and no one can make from a diff.

//go:embed hero.svg
var heroSVG string

// HeroPalette is the hero drawing's colors, by role.
//
// Frame is the outer border and the title-bar separator; Border is every inner
// rule and fill. They are the same value in the light theme and two different
// ones in the dark, which is why they cannot be one field.
type HeroPalette struct {
	Name       string // "light" | "dark", also the file suffix
	Accent     string
	Background string
	Panel      string
	Frame      string
	Border     string
	Selection  string
	Dim        string
	Text       string
	Green      string
	Amber      string
	Blue       string
}

// The values are the ones the committed assets already carried, so converting
// the drawing to a template changed no pixel. Whether the hero should adopt the
// animation's palette instead is a question about how it *looks*, and belongs to
// a change someone can see the result of.
var (
	heroLight = HeroPalette{
		Name:       "light",
		Accent:     "#0284c7",
		Background: "#ffffff",
		Panel:      "#f1f5f9",
		Frame:      "#e2e8f0",
		Border:     "#e2e8f0",
		Selection:  "#e0f2fe",
		Dim:        "#94a3b8",
		Text:       "#0f172a",
		Green:      "#16a34a",
		Amber:      "#b45309",
		Blue:       "#2563eb",
	}
	heroDark = HeroPalette{
		Name:       "dark",
		Accent:     "#38bdf8",
		Background: "#0b1017",
		Panel:      "#0e141c",
		Frame:      "#1b2430",
		Border:     "#1e293b",
		Selection:  "#16273c",
		Dim:        "#64748b",
		Text:       "#e2e8f0",
		Green:      "#4ade80",
		Amber:      "#fbbf24",
		Blue:       "#60a5fa",
	}
)

// HeroThemes are the palettes the hero ships in, in the order the files are written.
func HeroThemes() []HeroPalette { return []HeroPalette{heroLight, heroDark} }

// RenderHero draws the hero in one palette.
func RenderHero(p HeroPalette) (string, error) {
	t, err := template.New("hero").Option("missingkey=error").Parse(heroSVG)
	if err != nil {
		return "", fmt.Errorf("parsing the hero template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, p); err != nil {
		return "", fmt.Errorf("rendering the %s hero: %w", p.Name, err)
	}
	return b.String(), nil
}

// HeroTargets are the files a palette is written to. Both copies come from one
// render: they had drifted apart while each was maintained by hand.
func HeroTargets(p HeroPalette) []string {
	name := "hero.svg"
	if p.Name != "light" {
		name = "hero-" + p.Name + ".svg"
	}
	return []string{"docs/assets/" + name, "site/assets/" + name}
}
