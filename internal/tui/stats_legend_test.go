package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #845. The Stats tab's legend named six models while the chart drew four. Two of
// the entries — `—` and `<synthetic>` — had no pixel anywhere in the bars, and an
// operator reading the legend asks what those two models are.
//
// Measured on the live board, period `day`: 6 legend entries, 4 models carrying a
// token.
func legendDaily() []api.DailyStat {
	var daily []api.DailyStat
	// Old history, out of every short period's window.
	daily = append(daily,
		api.DailyStat{Day: "2026-08-05", Model: "claude-opus-4-8", OutputTokens: 900},
		// A marker Claude Code writes in an assistant line it generated itself, rolled
		// up before #433 filtered it and frozen in stats_daily ever since (#846).
		api.DailyStat{Day: "2026-08-05", Model: "<synthetic>", OutputTokens: 12879},
	)
	// Fifteen recent days, so the fourteen the `day` period shows exclude the above.
	for d := 1; d <= 15; d++ {
		day := fmt.Sprintf("2026-09-%02d", d)
		daily = append(daily,
			api.DailyStat{Day: day, Model: "claude-opus-5", OutputTokens: 1000, WorkingSeconds: 60},
			// The empty-model bucket is deliberate in the store — it means "not yet
			// known" and carries real status time — but it can never carry a token
			// (docs/design/token-rollup.md), so it has no place in a token chart.
			api.DailyStat{Day: day, Model: "", WorkingSeconds: 30, IdleSeconds: 90},
		)
	}
	return daily
}

func statsWith(daily []api.DailyStat, p period) string {
	return model{tab: tabStats, stat: statsView{period: p}, stats: api.StatsResponse{Daily: daily}}.renderStats()
}

// The legend describes the chart, so a model absent from the buckets on screen is
// absent from the legend. `statModels` read the whole history while the bars were
// truncated to the period, which is how a marker last seen in August was named
// under the `day` period.
func TestTheLegendNamesOnlyWhatTheChartDraws(t *testing.T) {
	out := statsWith(legendDaily(), periodDay)

	legend := legendLine(t, out)
	if strings.Contains(legend, "<synthetic>") {
		t.Errorf("legend = %q; it names a bucket whose only rows are out of the period", legend)
	}
	if strings.Contains(legend, "opus-4-8") {
		t.Errorf("legend = %q; it names a model the chart does not draw", legend)
	}
	if !strings.Contains(legend, "opus-5") {
		t.Errorf("legend = %q; it drops a model the chart does draw", legend)
	}
}

// A bucket that cannot hold a token has nothing to say in a token chart. It still
// belongs in the store, where it carries status time.
func TestTheLegendLeavesOutABucketWithNoTokens(t *testing.T) {
	out := statsWith(legendDaily(), periodDay)

	if legend := legendLine(t, out); strings.Contains(legend, "—") {
		t.Errorf("legend = %q; the empty-model bucket carries no tokens and draws nothing", legend)
	}
}

// Over a period that does include them, the same entries are legitimate: the bars
// carry them, so the legend must too. The rule is "what the chart draws", not a
// list of names to suppress.
func TestAModelInThePeriodStaysInTheLegend(t *testing.T) {
	out := statsWith(legendDaily(), periodTotal)

	if legend := legendLine(t, out); !strings.Contains(legend, "<synthetic>") {
		t.Errorf("legend = %q; over the total period that bucket does carry tokens", legend)
	}
}

// legendLine returns the line under the TOKENS heading, which is the legend.
func legendLine(t *testing.T, out string) string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.Contains(l, "TOKENS") && strings.Contains(l, "by model") && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	t.Fatalf("no TOKENS legend in:\n%s", out)
	return ""
}

// Filtering the legend shifts the indexes of everything after the entry removed,
// so a color keyed on position *in the legend* would change with the period. The
// palette is what an operator carries from one screen to the next, so it is keyed
// on something the period does not move.
//
// The premise is checked first: opus-5 really does sit at a different legend
// position under the two periods. Without that this test would pass on any
// implementation.
func TestAModelKeepsItsColorAcrossPeriods(t *testing.T) {
	daily := legendDaily()
	const mdl = "claude-opus-5"

	inDay := indexOf(statModels(bucketStats(daily, periodDay)), mdl)
	inTotal := indexOf(statModels(bucketStats(daily, periodTotal)), mdl)
	if inDay == inTotal {
		t.Fatalf("%s is at position %d under both periods; the fixture cannot show the defect", mdl, inDay)
	}
	if modelColor(inDay) == modelColor(inTotal) {
		t.Fatalf("positions %d and %d share a palette slot; the fixture cannot show the defect", inDay, inTotal)
	}

	if got, byPosition := colorForModel(daily, mdl), modelColor(inDay); got == byPosition && inDay != indexOf(allModels(daily), mdl) {
		t.Error("the color follows the legend position, so it moves when the period changes")
	}
	if colorForModel(daily, mdl) != modelColor(indexOf(allModels(daily), mdl)) {
		t.Error("the color is not keyed on the model's position among every model in the history")
	}
	if colorForModel(daily, mdl) == colorForModel(daily, "claude-opus-4-8") {
		t.Error("two models share a color; the stack cannot be read")
	}
}

func indexOf(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}

// allModels is the order colorForModel keys on, spelled out here so the test does
// not restate the implementation's loop.
func allModels(daily []api.DailyStat) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range daily {
		if !seen[d.Model] {
			seen[d.Model] = true
			out = append(out, d.Model)
		}
	}
	sort.Strings(out)
	return out
}
