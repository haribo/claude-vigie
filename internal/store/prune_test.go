package store

import (
	"context"
	"testing"
	"time"
)

func TestPruneSessions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	// fresh: reported recently → kept
	fresh := sampleSession("fresh")
	fresh.ReportedAt = rfc(1 * time.Hour)
	// stale: last report well beyond retention → pruned (with its samples)
	stale := sampleSession("stale")
	stale.ReportedAt = rfc(48 * time.Hour)
	// legacy: no report timestamp, old last_seen_at → pruned via fallback
	legacy := sampleSession("legacy")
	legacy.ReportedAt = ""
	legacy.LastSeenAt = rfc(48 * time.Hour)

	for _, s := range []Session{fresh, stale, legacy} {
		if err := st.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddSample(ctx, "stale", rfc(48*time.Hour), 10); err != nil {
		t.Fatal(err)
	}

	n, err := st.PruneSessions(ctx, 24*time.Hour, now)
	if err != nil {
		t.Fatalf("PruneSessions: %v", err)
	}
	if n != 2 {
		t.Errorf("pruned %d, want 2 (stale + legacy)", n)
	}
	if _, err := st.GetSession(ctx, "fresh"); err != nil {
		t.Errorf("fresh should be kept: %v", err)
	}
	if _, err := st.GetSession(ctx, "stale"); err == nil {
		t.Error("stale should be pruned")
	}
	if samples, _ := st.ListSamples(ctx, "stale", 10); len(samples) != 0 {
		t.Errorf("stale samples not pruned: %v", samples)
	}
}
