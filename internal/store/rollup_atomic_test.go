package store

import (
	"context"
	"testing"
)

// #669. The mark's meaning is "this much is already in stats_daily". Raising it
// with a separate call meant it could become true ahead of the fact: the mark
// advanced, the daily write failed, and since stats_daily is never recomputed the
// growth was gone for good — silently, with nothing recording which day needed
// repairing.
//
// The failure is induced by removing the table the second write needs. That is
// blunt, and it is the point: any error there must leave the mark untouched, so
// the next report counts the same growth again.
func TestRollUpTokensLeavesTheMarkAloneWhenTheDailyWriteFails(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.db.ExecContext(ctx, `DROP TABLE stats_daily`); err != nil {
		t.Fatal(err)
	}

	if _, err := st.RollUpTokens(ctx, "s1", 500, "2026-08-31", "opus"); err == nil {
		t.Fatal("no error from a rollup whose daily write cannot land")
	}

	var counted int64
	err := st.db.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT counted FROM session_token_mark WHERE session_id = 's1'), 0)`).Scan(&counted)
	if err != nil {
		t.Fatal(err)
	}
	if counted != 0 {
		t.Errorf("the mark advanced to %d while nothing was counted; that growth is lost for good", counted)
	}
}

// The working path, and the property the mark exists for: growth counts exactly
// once, however many reports carry the same total.
func TestRollUpTokensCountsGrowthExactlyOnce(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	const day, model = "2026-08-31", "opus"

	delta, err := st.RollUpTokens(ctx, "s1", 500, day, model)
	if err != nil || delta != 500 {
		t.Fatalf("first rollup = %d, %v; want 500", delta, err)
	}
	// The same total again: nothing new.
	if delta, err = st.RollUpTokens(ctx, "s1", 500, day, model); err != nil || delta != 0 {
		t.Fatalf("repeat rollup = %d, %v; want 0", delta, err)
	}
	// Growth: only the growth.
	if delta, err = st.RollUpTokens(ctx, "s1", 800, day, model); err != nil || delta != 300 {
		t.Fatalf("growth rollup = %d, %v; want 300", delta, err)
	}
	// A regression contributes nothing and does not lower the mark.
	if delta, err = st.RollUpTokens(ctx, "s1", 100, day, model); err != nil || delta != 0 {
		t.Fatalf("regression rollup = %d, %v; want 0", delta, err)
	}

	stats, err := st.ListDailyStats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range stats {
		total += r.OutputTokens
	}
	if total != 800 {
		t.Errorf("stats_daily holds %d output tokens, want 800", total)
	}
}
