package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestRenderUsageUnavailable(t *testing.T) {
	if out := renderUsage(api.UsageReport{}); !strings.Contains(out, "unavailable") {
		t.Errorf("empty usage should show unavailable:\n%s", out)
	}
}

func TestRenderUsageGauges(t *testing.T) {
	out := renderUsage(api.UsageReport{
		FiveHourPct: 13, FiveHourReset: "2026-07-27T11:00:00Z",
		SevenDayPct: 30, SevenDayReset: "2026-08-01T03:00:00Z",
		FetchedAt: "2026-07-27T10:00:00Z",
	})
	for _, want := range []string{"Current session (5h)", "This week", "13%", "30%", "resets", "synced"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage view missing %q:\n%s", want, out)
		}
	}
}

func TestResetLabelParsesFractional(t *testing.T) {
	frac := "2026-07-27T11:00:00.033544+00:00"
	if got := resetLabel(frac); got == frac {
		t.Errorf("resetLabel did not parse fractional-second time: %s", got)
	}
}
