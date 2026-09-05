package docs_test

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// #738. The landing page shows a legend of status pills — for many visitors the
// first place they learn what vigie's colors mean, before they install anything.
// Those colors were picked by hand when the page was written and matched
// nothing: `working` orange where the product is green, `idle` grey where it is
// blue, `compacting` a hue of its own where the product gives it `working`'s
// green. The legend also left out `error`, the one color that means drop what
// you are doing.
//
// The two clients are held to `test/fixtures/status-colors.json`
// (internal/tui/status_color_test.go). Nothing held the site, which is how it
// drifted this far without anyone noticing.

type statusColor struct {
	Status string `json:"status"`
	Family string `json:"family"`
	Light  string `json:"light"`
	Dark   string `json:"dark"`
}

func siteBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../site/index.html") //nolint:gosec // our own committed page
	if err != nil {
		t.Fatalf("reading the site: %v", err)
	}
	return string(b)
}

func statusPalette(t *testing.T) map[string]statusColor {
	t.Helper()
	b, err := os.ReadFile("../fixtures/status-colors.json") //nolint:gosec // our own committed fixture
	if err != nil {
		t.Fatalf("reading the palette: %v", err)
	}
	var doc struct {
		Statuses []statusColor `json:"statuses"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parsing the palette: %v", err)
	}
	out := make(map[string]statusColor, len(doc.Statuses))
	for _, s := range doc.Statuses {
		out[s.Status] = s
	}
	return out
}

// cssBlock returns the body of the declaration block opened by header. The page
// is dark-first: bare `:root` and `[data-theme="dark"]` carry the dark palette,
// the light media query and `[data-theme="light"]` carry the light one — so a
// color has to be right in four places, not one.
func cssBlock(t *testing.T, body, header string) string {
	t.Helper()
	i := strings.Index(body, header)
	if i < 0 {
		t.Fatalf("the site has no %q block — the page's theme structure changed and this test no longer measures it", header)
	}
	rest := body[i+len(header):]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("unterminated %q block", header)
	}
	return rest[:end]
}

var legendPill = regexp.MustCompile(`<span class="pill (\w+)"><i></i>(\w+)</span>`)

// Every pill in the legend names a real status and is painted its real color, in
// both themes.
func TestTheSiteLegendUsesTheProductColours(t *testing.T) {
	body := siteBody(t)
	palette := statusPalette(t)

	pills := legendPill.FindAllStringSubmatch(body, -1)
	if len(pills) == 0 {
		t.Fatal("no status pills found on the site — if the legend was removed, remove this test with it")
	}

	dark := []string{":root {", `:root[data-theme="dark"] {`}
	light := []string{"@media (prefers-color-scheme: light) {\n    :root {", `:root[data-theme="light"] {`}

	for _, p := range pills {
		class, label := p[1], p[2]
		if class != label {
			t.Errorf("pill class %q labeled %q — the legend says one status and paints another", class, label)
			continue
		}
		want, ok := palette[label]
		if !ok {
			t.Errorf("the legend shows %q, which is not a status vigie has", label)
			continue
		}
		decl := fmt.Sprintf("--s-%s: %s;", label, want.Dark)
		for _, header := range dark {
			if !strings.Contains(cssBlock(t, body, header), decl) {
				t.Errorf("%s: %q missing — the site paints %s something the product does not", header, decl, label)
			}
		}
		decl = fmt.Sprintf("--s-%s: %s;", label, want.Light)
		for _, header := range light {
			if !strings.Contains(cssBlock(t, body, header), decl) {
				t.Errorf("%s: %q missing — the light theme shows a color of its own for %s", header, decl, label)
			}
		}
	}
}

// The calling family is why the legend exists. A page that shows a board with no
// urgent state in it undersells the product and mis-teaches the color rule.
func TestTheSiteLegendShowsTheCallingFamily(t *testing.T) {
	body := siteBody(t)
	palette := statusPalette(t)

	matches := legendPill.FindAllStringSubmatch(body, -1)
	shown := make([]string, 0, len(matches))
	for _, p := range matches {
		shown = append(shown, p[2])
	}
	families := map[string]bool{}
	for _, s := range shown {
		if c, ok := palette[s]; ok {
			families[c.Family] = true
		}
	}
	for _, want := range []string{"running", "calling", "at rest", "over"} {
		if !families[want] {
			t.Errorf("no pill for the %q family; the legend shows %v", want, shown)
		}
	}
	if !contains(shown, "error") {
		t.Errorf("`error` is absent from %v — the calling family's two colors are two different asks", shown)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
