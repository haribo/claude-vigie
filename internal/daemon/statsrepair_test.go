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
