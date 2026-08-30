package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/haribo/claude-vigie/internal/status"
)

// One color per attention family, and the same one in both clients (#654).
//
// The two had already drifted before the rule existed: `thinking` was violet in
// the terminal and a *different* violet in the browser on a light theme, and the
// browser had invented a zinc for `stale` close enough to `ended` to separate
// nothing. Nobody could have noticed — the palettes lived in two files with
// nothing between them.

type colorFixture struct {
	Statuses []struct {
		Status string `json:"status"`
		Family string `json:"family"`
		Light  string `json:"light"`
		Dark   string `json:"dark"`
	} `json:"statuses"`
}

func TestTheTerminalPaletteMatchesTheSharedFixture(t *testing.T) {
	f := loadFixture[colorFixture](t, "status-colors.json")
	if len(f.Statuses) == 0 {
		t.Fatal("the shared fixture has no statuses — the extraction is broken, not the code")
	}
	for _, c := range f.Statuses {
		got, ok := statusStyle(c.Status).GetForeground().(lipgloss.AdaptiveColor)
		if !ok {
			t.Errorf("%s: the status color is not adaptive — a terminal's background decides which side shows", c.Status)
			continue
		}
		if got.Light != c.Light || got.Dark != c.Dark {
			t.Errorf("%s (%s family) = %s/%s, the shared fixture says %s/%s",
				c.Status, c.Family, got.Light, got.Dark, c.Light, c.Dark)
		}
	}
}

// The dashboard's half. Its palette is a stylesheet, which a node test cannot
// evaluate without a DOM — so it is read here rather than nowhere.
func TestTheDashboardPaletteMatchesTheSharedFixture(t *testing.T) {
	f := loadFixture[colorFixture](t, "status-colors.json")
	b, err := os.ReadFile("../../internal/web/static/app.css")
	if err != nil {
		t.Fatalf("reading the stylesheet: %v", err)
	}
	css := string(b)
	dark := strings.Index(css, "prefers-color-scheme: dark")
	if dark < 0 {
		t.Fatal("no dark block in app.css — the extraction is broken, not the stylesheet")
	}
	light, darkHalf := css[:dark], css[dark:]

	value := func(t *testing.T, block, name string) string {
		t.Helper()
		m := regexp.MustCompile(`--` + name + `:\s*(#[0-9a-fA-F]{3,8})`).FindStringSubmatch(block)
		if m == nil {
			t.Fatalf("app.css declares no --%s", name)
		}
		return strings.ToLower(m[1])
	}
	for _, c := range f.Statuses {
		if got := value(t, light, c.Status); got != strings.ToLower(c.Light) {
			t.Errorf("--%s (light) = %s, the shared fixture says %s", c.Status, got, c.Light)
		}
		if got := value(t, darkHalf, c.Status); got != strings.ToLower(c.Dark) {
			t.Errorf("--%s (dark) = %s, the shared fixture says %s", c.Status, got, c.Dark)
		}
	}
}

// The fixture must cover the vocabulary, or a status added tomorrow gets its
// color picked one at a time again — which is what the rule replaced.
func TestEveryStatusHasAFamily(t *testing.T) {
	f := loadFixture[colorFixture](t, "status-colors.json")
	got := map[string]bool{}
	for _, c := range f.Statuses {
		got[c.Status] = true
	}
	for _, st := range status.All {
		if !got[st] {
			t.Errorf("%q has no family in the shared fixture — its color would be picked on its own", st)
		}
	}
}
