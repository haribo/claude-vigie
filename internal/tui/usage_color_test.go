package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// #844. The same usage gauge was a different color depending on which client was
// open: at 98 % this one drew red while the browser drew amber — the color that
// decides whether the operator reacts.
//
// The rule now lives in docs/design/usage.md § 1bis and both clients implement it.
// The browser's half is `test/js/gauge_levels.test.mjs`; nothing keeps the two in
// step automatically, so the pair is reviewed together — the same standing caveat
// the scoped gauge carries (#840).
func TestAUsageGaugeChangesColorWhereTheDesignSaysItDoes(t *testing.T) {
	for _, c := range []struct {
		pct  float64
		want string
	}{
		{0, "green"}, {49.9, "green"},
		{50, "amber"}, {79.9, "amber"},
		{80, "red"}, {98, "red"}, {100, "red"},
	} {
		if got := colorName(usageColor(c.pct)); got != c.want {
			t.Errorf("at %.1f%% the gauge is %s, want %s (usage.md § 1bis)", c.pct, got, c.want)
		}
	}
}

// The levels are deliberately earlier than the Ctx column's, because a usage limit
// does not free itself the way a context does. Pinning the difference keeps a later
// "harmonize the thresholds" from quietly erasing the reason.
func TestUsageWarnsEarlierThanContext(t *testing.T) {
	const justUnderContextAmber = 55.0

	if colorName(usageColor(justUnderContextAmber)) != "amber" {
		t.Error("usage is not yet amber at 55%, so it warns no earlier than Ctx")
	}
	if colorName(contextColor(justUnderContextAmber)) != "green" {
		t.Error("Ctx is already amber at 55%; the premise of this test is gone")
	}
}

// colorName names one of the three palette entries a gauge can take, so a failure
// reads as "amber, want red" rather than as two hex pairs.
func colorName(c lipgloss.AdaptiveColor) string {
	switch c {
	case cRed:
		return "red"
	case cAmber:
		return "amber"
	case cGreen:
		return "green"
	}
	return "other"
}
