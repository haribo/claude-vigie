package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// #840. The strip showed `5h` and `7d` while Claude enforced a third limit, scoped
// to one model, which could be the highest of the three. An operator read a
// comfortable board while the limit that would stop the work sat at 98 %.

func stripWith(scoped *api.ScopedLimit) string {
	return renderUsageStrip(api.UsageReport{
		FetchedAt:     "2026-09-17T14:40:00Z",
		FiveHourPct:   21,
		FiveHourReset: "2026-09-17T17:30:00Z",
		SevenDayPct:   73,
		SevenDayReset: "2026-09-19T03:00:00Z",
		Scoped:        scoped,
	})
}

func TestTheScopedGaugeIsDrawnWithItsModelName(t *testing.T) {
	out := stripWith(&api.ScopedLimit{Label: "Fable", Pct: 98, Reset: "2026-09-19T03:00:00Z"})
	for _, want := range []string{"fable", "98%"} {
		if !strings.Contains(out, want) {
			t.Errorf("the strip does not carry %q:\n%s", want, out)
		}
	}
}

// Nothing is drawn and no space is reserved when no limit is scoped. A gauge
// permanently at zero trains the eye to skip the place where the exception appears
// — the reason `hidden N` is shown only when it has a value.
func TestNoScopedLimitLeavesTheStripAsItWas(t *testing.T) {
	before := stripWith(nil)
	if strings.Contains(before, "·") {
		t.Errorf("the strip reserved room for a gauge that does not exist:\n%s", before)
	}
}

// **The reset is written once because the two share a window, not because it is
// shorter.** When the endpoint ever reports different instants, both are printed —
// dropping one would then state something false.
func TestTheSharedResetIsWrittenOnceAndOnlyWhenItIsShared(t *testing.T) {
	same := stripWith(&api.ScopedLimit{Label: "Fable", Pct: 98, Reset: "2026-09-19T03:00:00Z"})
	if n := strings.Count(same, "("); n != 2 {
		t.Errorf("want two parenthesised resets (5h and the shared weekly one), got %d:\n%s", n, same)
	}

	diverged := stripWith(&api.ScopedLimit{Label: "Fable", Pct: 98, Reset: "2026-09-20T09:00:00Z"})
	if n := strings.Count(diverged, "("); n != 3 {
		t.Errorf("the two weekly windows reset at different times, so both must be shown; got %d:\n%s", n, diverged)
	}
}

// The TUI never scrolls sideways and this strip is already clamped, so the line has
// to fit a narrow terminal with the third gauge present.
func TestTheStripStillFitsANarrowTerminal(t *testing.T) {
	out := stripWith(&api.ScopedLimit{Label: "Fable", Pct: 98, Reset: "2026-09-19T03:00:00Z"})
	if w := lipgloss.Width(out); w > 80 {
		t.Errorf("the strip is %d columns wide, past an 80-column terminal:\n%s", w, out)
	}
}
