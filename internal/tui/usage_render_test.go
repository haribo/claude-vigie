package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestUsageStripUnavailable(t *testing.T) {
	if out := renderUsageStrip(api.UsageReport{}); !strings.Contains(out, "not fetched") {
		t.Errorf("empty usage should say not fetched:\n%s", out)
	}
}

func TestUsageStrip(t *testing.T) {
	out := renderUsageStrip(api.UsageReport{
		FiveHourPct: 13, FiveHourReset: time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		SevenDayPct: 30, SevenDayReset: time.Now().Add(4 * 24 * time.Hour).UTC().Format(time.RFC3339),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
	for _, want := range []string{"usage", "5h", "7d", "13%", "30%", "⟳"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage strip missing %q:\n%s", want, out)
		}
	}
}

func TestSyncGlyphFreshness(t *testing.T) {
	// A fresh snapshot renders a ⟳ glyph; an unparseable time still renders one.
	if g := syncGlyph(time.Now().UTC().Format(time.RFC3339)); !strings.Contains(g, "⟳") {
		t.Errorf("fresh syncGlyph = %q, want a ⟳", g)
	}
	if g := syncGlyph("not-a-time"); !strings.Contains(g, "⟳") {
		t.Errorf("unparseable syncGlyph = %q, want a ⟳", g)
	}
}

func TestResetInParsesFractional(t *testing.T) {
	// A future fractional-second time yields a non-empty remaining duration.
	frac := time.Now().Add(90 * time.Minute).UTC().Format("2006-01-02T15:04:05.000000Z07:00")
	if got := resetIn(frac); got == "" {
		t.Errorf("resetIn did not parse fractional-second time: %q", frac)
	}
}
