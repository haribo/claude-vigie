package tui

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #571. The hero is the README's lead image and the landing page's. It drew the
// TUI from before #492, #493 and #494 — the summary strip, five rules, eleven key
// hints, no state pill, and a bottom bar carrying `synced` and
// `platform ● operational`, both moved into the state modal. Nothing noticed for
// three releases, because nothing looked.
//
// #573 made it regenerable. This makes it checked, in the shape
// animation_asset_test.go already uses: the chrome *vocabulary* and the row
// count, never the pixels or the data. The asset is a drawing — its fleet, its
// ages and its token figures are its own, and comparing those would fail on every
// honest edit.

const heroAsset = "../../docs/assets/hero.svg"

// heroRows returns the drawn terminal rows, in y order — document order is not
// reading order and has been shuffled before.
func heroRows(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(heroAsset)
	if err != nil {
		t.Fatalf("reading the asset: %v", err)
	}
	re := regexp.MustCompile(`(?s)<text xml:space="preserve" x="18.0" y="([0-9.]+)">(.*?)</text>`)
	tags := regexp.MustCompile(`<[^>]+>`)
	type row struct {
		y float64
		s string
	}
	matches := re.FindAllStringSubmatch(string(b), -1)
	rows := make([]row, 0, len(matches))
	for _, m := range matches {
		y, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("unparseable y %q", m[1])
		}
		rows = append(rows, row{y, tags.ReplaceAllString(m[2], "")})
	}
	if len(rows) == 0 {
		t.Fatal("no terminal rows found: either the asset was redrawn in a different " +
			"structure — one <text> per row, x=18.0, xml:space=preserve — or this " +
			"extraction is stale. Both are failures; neither is the asset being fine.")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].y < rows[j].y })
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.s)
	}
	return out
}

// heroModel is the TUI the drawing depicts: the same eight columns and six
// sessions, at the width the asset is drawn to.
func heroModel() model {
	m := stubModel()
	m.width, m.height = 108, 40
	add := func(title, machine, dir, mdl, status string, out, total int64) {
		m.sessions = append(m.sessions, api.SessionView{
			ID: title, Title: title, Machine: machine, ProjectDir: "/p/" + dir, Model: mdl,
			Status: status, Usage: api.Usage{OutputTokens: out, InputTokens: total - out},
		})
	}
	add("refactor auth flow", "nebula", "api-gateway", "claude-opus-4-8", "working", 820000, 3100000)
	add("fix flaky e2e test", "orion", "web-app", "claude-sonnet-5", "waiting", 612000, 1400000)
	add("profile query plan", "forge", "analytics", "claude-opus-4-8", "working", 410000, 1900000)
	add("write migration 0011", "forge", "vigie", "claude-opus-4-8", "idle", 77000, 290000)
	add("review PR #204", "nebula", "infra", "claude-haiku-4-5", "idle", 31000, 120000)
	add("draft release notes", "orion", "docs-site", "claude-sonnet-5", "ended", 58000, 210000)
	m.prefs = defaultPrefs()
	m.prefs.hideEnded = false
	shown := map[string]bool{"name": true, "machine": true, "dir": true, "model": true,
		"out": true, "total": true, "seen": true, "status": true}
	m.prefs.columnOrder = []string{"name", "machine", "dir", "model", "out", "total", "seen", "status"}
	for _, c := range columns {
		if !shown[c.key()] {
			m.prefs.columnHidden = append(m.prefs.columnHidden, c.key())
		}
	}
	m.sseLive = true
	// Without a snapshot the strip reads "not fetched yet" and the two sides
	// disagree for a reason that has nothing to do with drift.
	m.usage = api.UsageReport{
		FiveHourPct: 38, FiveHourReset: "2026-08-18T14:40:00Z",
		SevenDayPct: 61, SevenDayReset: "2026-08-22T12:00:00Z",
		FetchedAt: "2026-08-18T11:57:00Z",
	}
	return m
}

// The count is the load-bearing half: a chrome row added or removed is exactly
// what went unnoticed for three releases, and it is measurable on both sides.
func TestTheHeroDrawsAsManyChromeRowsAsTheTuiRenders(t *testing.T) {
	const sessions = 6
	drawn := len(heroRows(t)) - sessions

	m := heroModel()
	rendered := lineCount(m.View()) - len(m.visibleSessions())

	if drawn != rendered {
		t.Errorf("the hero draws %d chrome rows and the TUI renders %d — the README's lead image shows a screen that no longer exists\n\nasset:\n  %s\n\nTUI:\n%s",
			drawn, rendered, strings.Join(heroRows(t), "\n  "), m.View())
	}
}

func TestTheHeroAndTheTuiAgreeOnTheChromeVocabulary(t *testing.T) {
	asset := strings.Join(heroRows(t), "\n")
	rendered := heroModel().View()

	for _, token := range []string{
		"Sessions", "Stats", "Machines", "Settings", // the tab line
		" i ",                                                               // the state modal's keycap
		"NAME", "MACHINE", "DIR", "MODEL", "OUT", "TOTAL", "SEEN", "STATUS", // the pinned header
		"usage", "5h", "7d", // the gauges
		"sort ",       // the view state
		" h ", "help", // the one key hint
	} {
		inAsset, inTUI := strings.Contains(asset, token), strings.Contains(rendered, token)
		switch {
		case inAsset && !inTUI:
			t.Errorf("the hero draws %q, which the TUI no longer renders", token)
		case !inAsset && inTUI:
			t.Errorf("the TUI renders %q, which the hero does not draw", token)
		}
	}

	// Deleted from the chrome, or never in the bottom bar. Neither side may carry
	// them — this is the exact list the hero was still drawing.
	for _, gone := range []string{"● working 3", "out 4.8M", "rc ◉", "synced ", "platform ", "⇥  switch"} {
		if strings.Contains(asset, gone) {
			t.Errorf("the hero still draws %q — a screen from before #492/#493/#494", gone)
		}
		if strings.Contains(rendered, gone) {
			t.Errorf("the TUI renders %q again — the hero and this guard both assume it is gone", gone)
		}
	}
}

// The gauges, as in the animation: the bar separated from its figure (#568), the
// reset tight against it (#492).
func TestTheHeroDrawsTheGaugesTheTuiRenders(t *testing.T) {
	asset := strings.Join(heroRows(t), "\n")
	if !regexp.MustCompile(`[░▓] \d+%`).MatchString(asset) {
		t.Errorf("the hero runs the bar into its percentage; the TUI separates them:\n%s", asset)
	}
	if !strings.Contains(asset, "%(") {
		t.Errorf("the hero does not draw the reset tight against the percentage:\n%s", asset)
	}
}
