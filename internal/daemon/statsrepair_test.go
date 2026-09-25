package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
)

func openStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(p)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, p
}

// TestStatsRepairReplacesOneBucket is the recovery path for #432: a production
// row held 61 051 295 773 output tokens where the real figure was 2 713 408, and
// nothing could correct it — stats_daily is never recomputed.
func TestStatsRepairReplacesOneBucket(t *testing.T) {
	st, path := openStore(t)
	ctx := context.Background()

	if err := st.AddDailyTokens(ctx, "2026-08-12", "claude-opus-4-8", 61_051_295_773); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyTokens(ctx, "2026-08-12", "other-model", 4_000); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyTokens(ctx, "2026-08-13", "claude-opus-4-8", 5_000); err != nil {
		t.Fatal(err)
	}

	code := runStatsRepair([]string{
		"-db", path, "-day", "2026-08-12", "-model", "claude-opus-4-8", "-output-tokens", "2713408",
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	rows, err := st.ListDailyStats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Day+"/"+r.Model] = r.OutputTokens
	}
	if got["2026-08-12/claude-opus-4-8"] != 2_713_408 {
		t.Errorf("repaired bucket = %d, want 2713408", got["2026-08-12/claude-opus-4-8"])
	}
	// Surgery on one row means exactly one row.
	if got["2026-08-12/other-model"] != 4_000 {
		t.Errorf("another model on the same day changed: %d", got["2026-08-12/other-model"])
	}
	if got["2026-08-13/claude-opus-4-8"] != 5_000 {
		t.Errorf("another day for the same model changed: %d", got["2026-08-13/claude-opus-4-8"])
	}
}

// TestStatsRepairRejectsNonsense: the command rewrites history that cannot be
// rebuilt, so it refuses anything it cannot interpret rather than guessing.
func TestStatsRepairRejectsNonsense(t *testing.T) {
	_, path := openStore(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no day", []string{"-db", path, "-output-tokens", "1"}},
		{"malformed day", []string{"-db", path, "-day", "12/08/2026", "-output-tokens", "1"}},
		{"day with time", []string{"-db", path, "-day", "2026-08-12T00:00:00Z", "-output-tokens", "1"}},
		{"no token count", []string{"-db", path, "-day", "2026-08-12"}},
		{"negative tokens", []string{"-db", path, "-day", "2026-08-12", "-output-tokens", "-5"}},
	} {
		if code := runStatsRepair(tc.args); code == 0 {
			t.Errorf("%s: exit = 0, want a refusal", tc.name)
		}
	}
}

// TestStatsRepairAcceptsZeroAndTheEmptyModel: zero is a legitimate correction for
// a bucket that should never have existed, and "" is a real bucket — reports that
// carried no model land in it.
func TestStatsRepairAcceptsZeroAndTheEmptyModel(t *testing.T) {
	st, path := openStore(t)
	ctx := context.Background()

	if err := st.AddDailyTokens(ctx, "2026-08-11", "", 12_879); err != nil {
		t.Fatal(err)
	}
	if code := runStatsRepair([]string{"-db", path, "-day", "2026-08-11", "-output-tokens", "0"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	rows, err := st.ListDailyStats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Day == "2026-08-11" && r.Model == "" && r.OutputTokens != 0 {
			t.Errorf("empty-model bucket = %d, want 0", r.OutputTokens)
		}
	}
}

// #846. `<synthetic>` is the marker Claude Code writes in an assistant line's
// `model` field for lines it generated itself; two days of August were rolled up
// before #433 filtered it, and the table is never recomputed. Zeroing the figure
// — all this command could do — leaves the row, its status seconds and its place
// in the chart's legend, and throws away tokens that belong to a real model.
func TestStatsRepairFoldsAPoisonedBucketIntoItsModel(t *testing.T) {
	st, path := openStore(t)
	ctx := context.Background()
	if err := st.AddDailyTokens(ctx, "2026-08-05", "<synthetic>", 12_879); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyStatusSeconds(ctx, "2026-08-05", "<synthetic>", "working", 13); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyTokens(ctx, "2026-08-05", "claude-opus-4-8", 900); err != nil {
		t.Fatal(err)
	}

	if code := runStatsRepair([]string{
		"-db", path, "-day", "2026-08-05", "-model", "<synthetic>", "-into", "claude-opus-4-8",
	}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	stats, err := st.ListDailyStats(ctx, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range stats {
		if d.Model == "<synthetic>" {
			t.Error("the marker bucket survived, so the chart still offers it as a model")
		}
		if d.Model == "claude-opus-4-8" && d.OutputTokens != 13_779 {
			t.Errorf("the real model holds %d tokens, want 13779 — the moved output is its own", d.OutputTokens)
		}
	}
}

// Deletion is for a bucket with nothing to return. It must say what it removed:
// nothing can be recomputed afterwards to check.
func TestStatsRepairDeletesABucketAndSaysWhatItHeld(t *testing.T) {
	st, path := openStore(t)
	ctx := context.Background()
	if err := st.AddDailyStatusSeconds(ctx, "2026-08-15", "<synthetic>", "working", 149); err != nil {
		t.Fatal(err)
	}

	if code := runStatsRepair([]string{
		"-db", path, "-day", "2026-08-15", "-model", "<synthetic>", "-delete",
	}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	stats, err := st.ListDailyStats(ctx, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range stats {
		if d.Model == "<synthetic>" {
			t.Error("the bucket survived the delete")
		}
	}
}

// One operation at a time. Two given together is an operator who meant one of
// them, on a table where the wrong write cannot be undone — and giving none must
// not fall through to a default that writes something.
func TestStatsRepairTakesExactlyOneOperation(t *testing.T) {
	_, path := openStore(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"none", []string{"-db", path, "-day", "2026-08-05", "-model", "x"}},
		{"tokens and delete", []string{"-db", path, "-day", "2026-08-05", "-model", "x", "-output-tokens", "1", "-delete"}},
		{"tokens and into", []string{"-db", path, "-day", "2026-08-05", "-model", "x", "-output-tokens", "1", "-into", "y"}},
		{"into and delete", []string{"-db", path, "-day", "2026-08-05", "-model", "x", "-into", "y", "-delete"}},
	} {
		if code := runStatsRepair(tc.args); code == 0 {
			t.Errorf("%s: exit = 0, want a refusal", tc.name)
		}
	}
}

// A day the operator mistyped must not quietly create an empty row under the
// destination's name, and must not report a move that did not happen.
func TestStatsRepairMovingAnAbsentBucketChangesNothing(t *testing.T) {
	st, path := openStore(t)
	ctx := context.Background()
	if err := st.AddDailyTokens(ctx, "2026-08-05", "claude-opus-5", 500); err != nil {
		t.Fatal(err)
	}

	if code := runStatsRepair([]string{
		"-db", path, "-day", "2026-08-06", "-model", "<synthetic>", "-into", "claude-opus-5",
	}); code != 0 {
		t.Fatalf("exit = %d, want 0 — an absent source is not an error", code)
	}

	stats, err := st.ListDailyStats(ctx, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].OutputTokens != 500 {
		t.Errorf("stats = %+v, want the one untouched row", stats)
	}
}
