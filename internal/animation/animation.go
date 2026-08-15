// Package animation renders the README's "A session can call you" asset.
//
// The SVG used to be produced once by a script that was never committed, so it
// could not be regenerated: a change to the TUI's rendering would leave it showing
// a product that no longer exists, and only a human noticing would catch it
// (#450).
//
// The drawing lives in template.svg and the two themes are the same template
// rendered with two palettes — the light and dark files cannot drift apart in
// structure, because there is only one structure. `animation_test.go` renders both
// and compares them to the committed files, so an edit to either side that is not
// carried through fails the build.
package animation

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed template.svg
var templateSVG string

// Themes are the palettes the asset ships in, in the order the files are written.
func Themes() []Palette { return []Palette{lightPalette, darkPalette} }

// Render draws the asset in one palette.
//
// text/template rather than html/template on purpose: the output is SVG, the
// inputs are our own color literals, and HTML escaping would corrupt the `&gt;`
// entities and the box-drawing characters the terminal chrome is made of.
func Render(p Palette) (string, error) {
	t, err := template.New("animation").Option("missingkey=error").Parse(templateSVG)
	if err != nil {
		return "", fmt.Errorf("parsing the animation template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, p); err != nil {
		return "", fmt.Errorf("rendering the %s animation: %w", p.Name, err)
	}
	return b.String(), nil
}

// Targets are every file the asset is written to: the README's copy and the
// site's. They are listed here so nothing has to remember the second one — which
// is how two copies of an asset drift apart.
func Targets(p Palette) []string {
	name := "session-call.svg"
	if p.Name != "light" {
		name = "session-call-" + p.Name + ".svg"
	}
	return []string{"docs/assets/" + name, "site/assets/" + name}
}
