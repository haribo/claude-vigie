package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #505. The README's animation showed a Sessions tab that no longer existed: the
// status-count row and its rule had been deleted, the usage gauges tightened, and
// the state pill added, all while five tests on the asset stayed green.
//
// Every one of those five checks internal consistency — that the four rendered
// files agree with each other and with the template they came from. None asks
// whether the template depicts the product, so the drawing could drift
// arbitrarily far from the TUI with a fully green build. That is the same failure
// shape as the metrics paragraph in deployment.md (#478): checked for structure,
// never for truth.
//
// This test lives in package tui rather than beside the other asset checks
// because it needs the real renderer, whose helpers are unexported.
//
// WHAT IT DOES NOT COVER. The asset is a drawing, not a screenshot: its data,
// colors, spacing and layout are its own, and none of that is compared here.
// What is compared is the *vocabulary of the chrome* — how many rows frame the
// table, and which chrome elements are present or absent. It fails on exactly the
// class of change that broke it: an element added to or removed from the chrome.
// A reader must not mistake it for a full comparison.

const animationAsset = "../../docs/assets/session-call.svg"

// assetRows returns the plain text of the asset's "after" panel, one string per
// drawn row, tags stripped.
func assetRows(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(animationAsset)
	if err != nil {
		t.Fatalf("reading the asset: %v", err)
	}
	s := string(body)
	i := strings.Index(s, `<g class="after">`)
	if i < 0 {
		t.Fatal("the asset has no `after` panel — this guard needs updating")
	}
	after := s[i:]
	if j := strings.Index(after, "</g>"); j >= 0 {
		after = after[:j]
	}
	texts := regexp.MustCompile(`(?s)<text[^>]*>(.*?)</text>`).FindAllStringSubmatch(after, -1)
	out := make([]string, 0, len(texts))
	for _, m := range texts {
		out = append(out, regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m[1], ""))
	}
	if len(out) == 0 {
		t.Fatal("no rows found in the asset — the extraction is broken, not the asset")
	}
	return out
}

// assetModel is the TUI rendering the asset depicts: the same four columns, the
// same three sessions, wide enough that no column overflows.
func assetModel() model {
	m := stubModel()
	m.width, m.height = 100, 40
	m.sessions = []api.SessionView{
		{ID: "a", Title: "api-gateway", ProjectDir: "/p/api-gateway", Status: "working"},
		{ID: "b", Title: "web-app", ProjectDir: "/p/web-app", Status: "idle", CallAt: "2026-08-16T12:00:00Z"},
		{ID: "c", Title: "data-pipeline", ProjectDir: "/p/data-pipeline", Status: "waiting"},
	}
	m.prefs = defaultPrefs()
	m.prefs.hideEnded = false
	m.prefs.columnOrder = []string{"name", "dir", "status", "detail"}
	for _, c := range columns {
		switch c.key() {
		case "name", "dir", "status", "detail":
		default:
			m.prefs.columnHidden = append(m.prefs.columnHidden, c.key())
		}
	}
	m.sseLive = true
	// The asset draws both gauges, so the model must have a snapshot to draw them
	// from — otherwise the bar reads "usage — not fetched yet" and the two sides
	// disagree for a reason that has nothing to do with drift.
	m.usage = api.UsageReport{
		FiveHourPct: 28, FiveHourReset: "2026-08-16T16:00:00Z",
		SevenDayPct: 69, SevenDayReset: "2026-08-18T15:00:00Z",
		FetchedAt: "2026-08-16T15:00:00Z",
	}
	return m
}

// The count is the load-bearing half: a chrome row added or removed is exactly
// what made the asset stale, and it is measurable on both sides.
func TestTheAssetDrawsAsManyChromeRowsAsTheTuiRenders(t *testing.T) {
	const sessions = 3
	drawn := len(assetRows(t)) - sessions

	m := assetModel()
	rendered := lineCount(m.View()) - len(m.visibleSessions())

	if drawn != rendered {
		t.Errorf("the asset draws %d chrome rows and the TUI renders %d — the README shows a screen that no longer exists\n\nasset:\n  %s\n\nTUI:\n%s",
			drawn, rendered, strings.Join(assetRows(t), "\n  "), m.View())
	}
}

// And the vocabulary: what the asset shows, the TUI must show, and the reverse.
func TestTheAssetAndTheTuiAgreeOnTheChromeVocabulary(t *testing.T) {
	asset := strings.Join(assetRows(t), "\n")
	rendered := assetModel().View()

	// Chrome elements, not data. Each must be in both or in neither.
	for _, token := range []string{
		"Sessions", "Stats", "Machines", "Settings", // the tab line
		" i ",                             // the state modal's keycap, in the corner
		"NAME", "DIR", "STATUS", "DETAIL", // the pinned column header
		"usage", "5h", "7d", // the gauges
		"sort ",       // the view state
		" h ", "help", // the one key hint
	} {
		inAsset, inTUI := strings.Contains(asset, token), strings.Contains(rendered, token)
		switch {
		case inAsset && !inTUI:
			t.Errorf("the asset draws %q, which the TUI no longer renders", token)
		case !inAsset && inTUI:
			t.Errorf("the TUI renders %q, which the asset does not draw", token)
		}
	}

	// Elements that were deleted from the chrome. Neither side may carry them, or
	// the asset is depicting a screen from before the change.
	for _, gone := range []string{"● working 2", "● ended 0", "rc ◉", "activity "} {
		if strings.Contains(asset, gone) {
			t.Errorf("the asset still draws %q, deleted from the chrome in #492", gone)
		}
		if strings.Contains(rendered, gone) {
			t.Errorf("the TUI renders %q again — the asset and this guard both assume it is gone", gone)
		}
	}
}

// The gauges were tightened against their percentage and their reset (#492); an
// asset drawn from the old spacing is the drift this issue is about.
func TestTheAssetDrawsTheGaugesTheTuiRenders(t *testing.T) {
	asset := strings.Join(assetRows(t), "\n")
	if regexp.MustCompile(`░\s+\d+%`).MatchString(asset) {
		t.Errorf("the asset still puts a gap between the bar and its percentage:\n%s", asset)
	}
	if !strings.Contains(asset, "%(") {
		t.Errorf("the asset does not draw the reset tight against the percentage:\n%s", asset)
	}
}
