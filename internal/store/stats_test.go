package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newStatsStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestDailyTokensAccumulate(t *testing.T) {
	st := newStatsStore(t)
	ctx := context.Background()

	if err := st.AddDailyTokens(ctx, "2026-07-29", "opus", 100); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyTokens(ctx, "2026-07-29", "opus", 50); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyTokens(ctx, "2026-07-29", "sonnet", 30); err != nil {
		t.Fatal(err)
	}
	// Non-positive deltas are ignored.
	if err := st.AddDailyTokens(ctx, "2026-07-29", "opus", 0); err != nil {
		t.Fatal(err)
	}

	rows, err := st.ListDailyStats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Model] = r.OutputTokens
	}
	if got["opus"] != 150 {
		t.Errorf("opus tokens = %d, want 150", got["opus"])
	}
	if got["sonnet"] != 30 {
		t.Errorf("sonnet tokens = %d, want 30", got["sonnet"])
	}
}

func TestDailyStatusSeconds(t *testing.T) {
	st := newStatsStore(t)
	ctx := context.Background()

	if err := st.AddDailyStatusSeconds(ctx, "2026-07-29", "opus", "waiting", 120); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyStatusSeconds(ctx, "2026-07-29", "opus", "waiting", 60); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyStatusSeconds(ctx, "2026-07-29", "opus", "working", 300); err != nil {
		t.Fatal(err)
	}
	// Unknown status and non-positive durations are ignored.
	if err := st.AddDailyStatusSeconds(ctx, "2026-07-29", "opus", "ended", 999); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyStatusSeconds(ctx, "2026-07-29", "opus", "idle", -5); err != nil {
		t.Fatal(err)
	}

	rows, err := st.ListDailyStats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.WaitingSeconds != 180 || r.WorkingSeconds != 300 || r.IdleSeconds != 0 {
		t.Errorf("got waiting=%d working=%d idle=%d, want 180/300/0",
			r.WaitingSeconds, r.WorkingSeconds, r.IdleSeconds)
	}
}

func TestListDailyStatsSince(t *testing.T) {
	st := newStatsStore(t)
	ctx := context.Background()
	for _, day := range []string{"2026-07-27", "2026-07-28", "2026-07-29"} {
		if err := st.AddDailyTokens(ctx, day, "opus", 10); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListDailyStats(ctx, "2026-07-28")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows since 2026-07-28 = %d, want 2", len(rows))
	}
	if rows[0].Day != "2026-07-28" {
		t.Errorf("first day = %s, want 2026-07-28 (oldest first)", rows[0].Day)
	}
}

func TestLastEvent(t *testing.T) {
	st := newStatsStore(t)
	ctx := context.Background()

	if _, ok, err := st.LastEvent(ctx, "missing"); err != nil || ok {
		t.Fatalf("LastEvent(missing) = ok %v, err %v; want false, nil", ok, err)
	}

	if err := st.UpsertSession(ctx, Session{ID: "s1", Machine: "m", Status: "idle", StartedAt: "t0", LastSeenAt: "t0"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []Event{
		{SessionID: "s1", Event: "UserPromptSubmit", Status: "working", CreatedAt: "2026-07-29T10:00:00Z"},
		{SessionID: "s1", Event: "Notification", Status: "waiting", CreatedAt: "2026-07-29T10:05:00Z"},
	} {
		if err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	last, ok, err := st.LastEvent(ctx, "s1")
	if err != nil || !ok {
		t.Fatalf("LastEvent(s1) ok %v err %v", ok, err)
	}
	if last.Status != "waiting" || last.CreatedAt != "2026-07-29T10:05:00Z" {
		t.Errorf("last event = %+v, want the waiting one", last)
	}
}
