package store

import (
	"context"
	"testing"
)

// #846. `<synthetic>` is not a model — it is the marker Claude Code writes in an
// assistant line's `model` field for lines it generated itself. The parser has
// filtered it since #433, but two days of August were rolled up before that and
// are frozen in `stats_daily`, which is never recomputed. One of them holds
// **12 879 real output tokens**, taken from whichever model actually produced
// them.
//
// `SetDailyTokens` can only rewrite the figure, so zeroing it leaves the row, its
// status seconds and its legend entry, and throws the tokens away rather than
// returning them to their model. These two operations are what that needs.

func seedBucket(t *testing.T, st *Store, day, model string, tokens, working int64) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := st.SetDailyTokens(ctx, day, model, tokens); err != nil {
		t.Fatal(err)
	}
	if working > 0 {
		if err := st.AddDailyStatusSeconds(ctx, day, model, "working", working); err != nil {
			t.Fatal(err)
		}
	}
}

func bucket(t *testing.T, st *Store, day, model string) (DailyStat, bool) {
	t.Helper()
	d, ok, err := readDailyBucket(context.Background(), st.db, day, model)
	if err != nil {
		t.Fatal(err)
	}
	return d, ok
}

func TestDeletingABucketRemovesItAndReportsWhatItHeld(t *testing.T) {
	st := openTestStore(t)
	seedBucket(t, st, "2026-08-15", "<synthetic>", 0, 149)

	before, existed, err := st.DeleteDailyBucket(context.Background(), "2026-08-15", "<synthetic>")

	if err != nil || !existed {
		t.Fatalf("delete: existed=%v err=%v", existed, err)
	}
	if before.WorkingSeconds != 149 {
		t.Errorf("reported %d working seconds, want 149 — the operator has to see what was removed", before.WorkingSeconds)
	}
	if _, ok := bucket(t, st, "2026-08-15", "<synthetic>"); ok {
		t.Error("the row survived the delete, so the chart still carries a bucket that is not a model")
	}
}

// Deleting something that is not there changes nothing and says so, rather than
// reporting a success the operator would read as "it was there and now it is not".
func TestDeletingAnAbsentBucketSaysSo(t *testing.T) {
	st := openTestStore(t)

	_, existed, err := st.DeleteDailyBucket(context.Background(), "2026-08-15", "nope")

	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Error("an absent bucket reported as existing")
	}
}

// The tokens in a poisoned bucket are real output. Moving them is what the 2026-08-05
// row needs: 12 879 tokens that belong to whichever model was running.
func TestMovingABucketFoldsItIntoTheDestination(t *testing.T) {
	st := openTestStore(t)
	seedBucket(t, st, "2026-08-05", "<synthetic>", 12879, 13)
	seedBucket(t, st, "2026-08-05", "claude-opus-4-8", 900, 100)

	before, existed, err := st.MoveDailyBucket(context.Background(), "2026-08-05", "<synthetic>", "claude-opus-4-8")

	if err != nil || !existed {
		t.Fatalf("move: existed=%v err=%v", existed, err)
	}
	if before.OutputTokens != 12879 {
		t.Errorf("reported %d tokens moved, want 12879", before.OutputTokens)
	}
	if _, ok := bucket(t, st, "2026-08-05", "<synthetic>"); ok {
		t.Error("the source survived, so its figures are now counted twice")
	}
	dst, ok := bucket(t, st, "2026-08-05", "claude-opus-4-8")
	if !ok {
		t.Fatal("the destination is gone")
	}
	if dst.OutputTokens != 13779 {
		t.Errorf("destination holds %d tokens, want 13779 (900 + 12879)", dst.OutputTokens)
	}
	if dst.WorkingSeconds != 113 {
		t.Errorf("destination holds %d working seconds, want 113 (100 + 13)", dst.WorkingSeconds)
	}
}

// A destination that does not exist yet is created, rather than the move failing
// on a day where the real model has no row of its own.
func TestMovingIntoAModelWithNoRowYetCreatesIt(t *testing.T) {
	st := openTestStore(t)
	seedBucket(t, st, "2026-08-05", "<synthetic>", 500, 9)

	if _, _, err := st.MoveDailyBucket(context.Background(), "2026-08-05", "<synthetic>", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}

	dst, ok := bucket(t, st, "2026-08-05", "claude-opus-5")
	if !ok || dst.OutputTokens != 500 || dst.WorkingSeconds != 9 {
		t.Errorf("destination = %+v exists=%v, want the source's figures", dst, ok)
	}
}

// Folding a bucket into itself would add its figures to themselves. Refused rather
// than performed, because this table can never be recomputed to undo it.
func TestABucketCannotBeMovedIntoItself(t *testing.T) {
	st := openTestStore(t)
	seedBucket(t, st, "2026-08-05", "claude-opus-5", 500, 9)

	if _, _, err := st.MoveDailyBucket(context.Background(), "2026-08-05", "claude-opus-5", "claude-opus-5"); err == nil {
		t.Fatal("a bucket was folded into itself")
	}

	if d, _ := bucket(t, st, "2026-08-05", "claude-opus-5"); d.OutputTokens != 500 {
		t.Errorf("tokens = %d after the refused move, want 500 untouched", d.OutputTokens)
	}
}

// Moving a bucket that is not there leaves the destination alone: a typo in the
// source must not create an empty row under the destination's name.
func TestMovingAnAbsentBucketTouchesNothing(t *testing.T) {
	st := openTestStore(t)
	seedBucket(t, st, "2026-08-05", "claude-opus-5", 500, 9)

	_, existed, err := st.MoveDailyBucket(context.Background(), "2026-08-05", "typo", "claude-opus-5")

	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Error("an absent source reported as existing")
	}
	if d, _ := bucket(t, st, "2026-08-05", "claude-opus-5"); d.OutputTokens != 500 {
		t.Errorf("destination tokens = %d, want 500 untouched", d.OutputTokens)
	}
}
