package tui

import (
	"regexp"
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
	for _, want := range []string{"usage", "5h", "7d", "13%", "30%"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage strip missing %q:\n%s", want, out)
		}
	}
}

func TestResetInParsesFractional(t *testing.T) {
	// A future fractional-second time yields a non-empty remaining duration.
	frac := time.Now().Add(90 * time.Minute).UTC().Format("2006-01-02T15:04:05.000000Z07:00")
	if got := resetIn(frac); got == "" {
		t.Errorf("resetIn did not parse fractional-second time: %q", frac)
	}
}

// #568. The strip spent its spacing between the two gauges instead of inside
// them: `░░4%` read as one block, because the fill glyph and the digit carry the
// same visual weight with nothing between them, so the eye had to hunt for where
// the bar ended. Four spaces meanwhile separated `5h` from `7d` — two elements of
// the same nature, already told apart by their own labels.
//
// The space is worth more inside a group than between groups. These assertions
// are on the shape rather than on a golden string, so a later color or glyph
// change does not have to touch them.
func usageStripFixture(t *testing.T) string {
	t.Helper()
	return renderUsageStrip(api.UsageReport{
		FiveHourPct: 4, FiveHourReset: time.Now().Add(4 * time.Hour).UTC().Format(time.RFC3339),
		SevenDayPct: 31, SevenDayReset: time.Now().Add(4 * 24 * time.Hour).UTC().Format(time.RFC3339),
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func TestTheBarIsSeparatedFromItsFigure(t *testing.T) {
	out := usageStripFixture(t)
	if m := regexp.MustCompile(`[░▓]\d`).FindString(out); m != "" {
		t.Errorf("the bar runs straight into its percentage (%q) — the eye cannot find where it ends:\n%s", m, out)
	}
}

func TestTheGaugesAreNotSeparatedMoreThanTheirParts(t *testing.T) {
	out := usageStripFixture(t)
	if strings.Contains(out, "    7d") {
		t.Errorf("four spaces before `7d`, more than inside a gauge — the space is worth more within a group:\n%s", out)
	}
	if !strings.Contains(out, "  7d") {
		t.Errorf("the two gauges must still be told apart:\n%s", out)
	}
}

func TestTheResetStaysGluedToItsPercentage(t *testing.T) {
	// It qualifies the figure; a space would make it look like a third element.
	out := usageStripFixture(t)
	if !regexp.MustCompile(`\d%\(`).MatchString(out) {
		t.Errorf("the reset hint detached from its percentage:\n%s", out)
	}
}

func TestTheStripDoesNotGrow(t *testing.T) {
	// The bottom bar shares one width budget between its halves (#486); this
	// change must not spend it. One space is given back by the label, two by the
	// separator, one taken by each gauge.
	if n := len([]rune(usageStripFixture(t))); n > 49 {
		t.Errorf("the strip is %d columns, was 50 before #568 and must not grow:\n%s", n, usageStripFixture(t))
	}
}
