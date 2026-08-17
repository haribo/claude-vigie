package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The deployment guide told operators the metrics were namespaced `fleet_*`,
// with a `fleet_sessions` gauge. The server has emitted `vigie_*` since the
// rename (#262/#263). An operator who wrote scrape rules or alerts from that
// paragraph collected nothing, forever, and nothing would have said so — a
// Prometheus query for a series that does not exist is not an error, it is an
// empty result (#478).
//
// So this checks the property that matters rather than the brand: every metric
// name a document or the shipped dashboard mentions must be one the code
// actually registers. It catches a stale prefix, a renamed metric, and a deleted
// one alike.
func TestEveryDocumentedMetricExists(t *testing.T) {
	registered := registeredMetrics(t)
	if len(registered) < 10 {
		t.Fatalf("only %d metrics found in metrics.go — the extraction is broken, not the docs", len(registered))
	}

	// A histogram registers one name and exposes three; a counter's _total is
	// part of the registered name already.
	suffixes := []string{"_bucket", "_sum", "_count"}
	mentioned := regexp.MustCompile(`\b(?:vigie|fleet)_[a-z_]+\b`)

	dirs := []string{"../../docs", "../../docs/design", "../../docs/adr"}
	sources := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		sources = append(sources, markdownIn(t, dir)...)
	}
	sources = append(sources, "../../README.md", "../../dashboards/vigie.json")
	for _, p := range sources {
		for _, name := range mentioned.FindAllString(read(t, p), -1) {
			base := name
			for _, s := range suffixes {
				base = strings.TrimSuffix(base, s)
			}
			if !registered[base] {
				t.Errorf("%s mentions the metric %q, which the server does not emit — a scrape built on it collects nothing",
					filepath.Base(p), name)
			}
		}
	}
}

// registeredMetrics reads the metric names out of the one file that defines
// them. Both declaration forms are covered: the Namespace/Name pair used by the
// collectors, and the namespace+"_name" concatenation used by the scrape-time
// Descs.
func registeredMetrics(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("../../internal/server/metrics.go")
	if err != nil {
		t.Fatal(err)
	}
	ns := regexp.MustCompile(`metricsNamespace = "([a-z]+)"`).FindSubmatch(src)
	if ns == nil {
		t.Fatal("metrics.go no longer declares metricsNamespace — this guard needs updating")
	}
	prefix := string(ns[1]) + "_"

	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`Name:\s*"([a-z_]+)"`).FindAllSubmatch(src, -1) {
		out[prefix+string(m[1])] = true
	}
	for _, m := range regexp.MustCompile(`metricsNamespace\+"(_[a-z_]+)"`).FindAllSubmatch(src, -1) {
		out[string(ns[1])+string(m[1])] = true
	}
	return out
}

// The other half of #478: identifiers shipped inside the two browser-side
// artifacts. `FleetIndicator` and four `cf-*` CSS classes survived the rename
// because nothing reads them but GNOME Shell itself — no compiler, no linter,
// and no test. The web dashboard's `cf_token` / `cf_columns` storage keys are
// the same, with one difference: they hold live state, so they are renamed *and*
// carried over (adoptLegacyKey), and the old names legitimately remain in that
// migration.
func TestTheShippedFrontEndsCarryNoOldBrand(t *testing.T) {
	brand := regexp.MustCompile(`\bFleet[A-Z]|\bcf-[a-z]|"cf_[a-z]`)
	// The one legitimate mention: the migration that reads the old keys once.
	legacy := regexp.MustCompile(`adoptLegacyKey\(localStorage, "cf_[a-z]+"`)

	for _, p := range []string{
		"../../gnome-extension/extension.js",
		"../../gnome-extension/prefs.js",
		"../../gnome-extension/lib.js",
		"../../gnome-extension/stylesheet.css",
		"../../internal/web/static/app.js",
		"../../internal/web/static/lib.js",
		"../../internal/web/static/app.css",
	} {
		body := read(t, p)
		for _, hit := range brand.FindAllStringIndex(body, -1) {
			line := body[lineStart(body, hit[0]):lineEnd(body, hit[1])]
			if legacy.MatchString(line) {
				continue // the read-time migration, kept on purpose
			}
			t.Errorf("%s still carries the old brand: %s", filepath.Base(p), strings.TrimSpace(line))
		}
	}
}

// A CSS class renamed in one file and not the other fails silently: GNOME Shell
// applies no style and reports nothing, so the badge simply stops looking like a
// badge. Every class the indicator asks for must exist in the stylesheet it
// ships with.
func TestEveryStyleClassTheIndicatorUsesExists(t *testing.T) {
	css := read(t, "../../gnome-extension/stylesheet.css")
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`\.([a-z][a-z0-9-]*)\s*\{`).FindAllStringSubmatch(css, -1) {
		defined[m[1]] = true
	}
	if len(defined) == 0 {
		t.Fatal("no class found in stylesheet.css — the extraction is broken")
	}

	js := read(t, "../../gnome-extension/extension.js")
	// Only our own classes: panel-status-menu-box and system-status-icon are
	// GNOME Shell's, defined in the shell's theme rather than here.
	used := regexp.MustCompile(`style_class(?:_name)?[:(]\s*'(vigie-[a-z0-9-]+)'`)
	seen := 0
	for _, m := range used.FindAllStringSubmatch(js, -1) {
		seen++
		if !defined[m[1]] {
			t.Errorf("extension.js styles with %q, which stylesheet.css does not define — the style silently does nothing", m[1])
		}
	}
	if seen == 0 {
		t.Error("no styled class found in extension.js — this guard stopped guarding anything")
	}
}

func lineStart(s string, i int) int {
	if j := strings.LastIndexByte(s[:i], '\n'); j >= 0 {
		return j + 1
	}
	return 0
}

func lineEnd(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(s)
}
