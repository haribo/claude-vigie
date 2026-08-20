package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestSamplesRoundtripAndRetention(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if last, err := st.LastSampleAt(ctx, "s1"); err != nil || last != "" {
		t.Fatalf("LastSampleAt empty = (%q, %v), want empty", last, err)
	}

	// Add more than the retention window; only the last sampleRetention survive.
	for i := 0; i < sampleRetention+5; i++ {
		at := fmt.Sprintf("2026-07-27T10:%02d:00Z", i)
		if err := st.AddSample(ctx, "s1", at, int64(i*100)); err != nil {
			t.Fatalf("AddSample: %v", err)
		}
	}

	samples, err := st.ListSamples(ctx, "s1", "", 100)
	if err != nil {
		t.Fatalf("ListSamples: %v", err)
	}
	if len(samples) != sampleRetention {
		t.Fatalf("retained %d samples, want %d", len(samples), sampleRetention)
	}
	// oldest-first, and the oldest kept is index 5 (0..4 pruned)
	if samples[0] != int64(5*100) {
		t.Errorf("oldest kept = %d, want %d", samples[0], 5*100)
	}
	if samples[len(samples)-1] != int64((sampleRetention+4)*100) {
		t.Errorf("newest = %d, want %d", samples[len(samples)-1], (sampleRetention+4)*100)
	}

	if last, err := st.LastSampleAt(ctx, "s1"); err != nil || last == "" {
		t.Errorf("LastSampleAt after adds = (%q, %v), want non-empty", last, err)
	}
}

func TestListSamplesSince(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	if err := st.AddSample(ctx, "s2", "2026-07-28T10:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSample(ctx, "s2", "2026-07-28T12:00:00Z", 200); err != nil {
		t.Fatal(err)
	}
	// Only the sample after the cutoff is returned.
	got, err := st.ListSamples(ctx, "s2", "2026-07-28T11:00:00Z", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 200 {
		t.Errorf("since-filtered = %v, want [200]", got)
	}
	// A cutoff after everything returns nothing (idle session → empty ACT).
	if got, _ := st.ListSamples(ctx, "s2", "2026-07-28T13:00:00Z", 100); len(got) != 0 {
		t.Errorf("all-stale = %v, want empty", got)
	}
}

// TestRecentSamplesAgreesWithListSamples guards #580: the batched read replaces
// the per-session one on the busiest path in the daemon, so the two must return
// the same thing for every session. Comparing against the implementation it
// replaces is stronger than restating the expected values — a shared mistake in
// both would have to be a mistake in the SQL of each, written differently.
func TestRecentSamplesAgreesWithListSamples(t *testing.T) {
	st, ctx := openTestStore(t), context.Background()

	// Two busy sessions and a quiet one, so the per-session cap has something to
	// cap and the quiet session must survive beside a busy neighbor.
	for i := range 40 {
		at := fmt.Sprintf("2026-07-28T10:%02d:00Z", i)
		if err := st.AddSample(ctx, "busy-a", at, int64(100+i)); err != nil {
			t.Fatal(err)
		}
		if err := st.AddSample(ctx, "busy-b", at, int64(900+i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddSample(ctx, "quiet", "2026-07-28T10:05:00Z", 7); err != nil {
		t.Fatal(err)
	}

	const since, limit = "2026-07-28T09:00:00Z", 30
	batched, err := st.RecentSamples(ctx, since, limit)
	if err != nil {
		t.Fatalf("RecentSamples: %v", err)
	}
	for _, id := range []string{"busy-a", "busy-b", "quiet"} {
		want, err := st.ListSamples(ctx, id, since, limit)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(batched[id], want) {
			t.Errorf("%s: batched %v, per-session %v", id, batched[id], want)
		}
	}
	// The cap is per session, not per result set: 30 each, not 30 shared.
	if len(batched["busy-a"]) != limit || len(batched["busy-b"]) != limit {
		t.Errorf("caps are not per session: busy-a %d, busy-b %d, want %d each",
			len(batched["busy-a"]), len(batched["busy-b"]), limit)
	}
	if len(batched["quiet"]) != 1 {
		t.Errorf("the quiet session was crowded out: %v", batched["quiet"])
	}
}

// A session with nothing new is absent rather than present-and-empty, and the
// caller reads a missing key as no samples.
func TestRecentSamplesHonorsSince(t *testing.T) {
	st, ctx := openTestStore(t), context.Background()
	if err := st.AddSample(ctx, "old", "2026-07-28T10:00:00Z", 5); err != nil {
		t.Fatal(err)
	}
	got, err := st.RecentSamples(ctx, "2026-07-28T12:00:00Z", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("samples older than since came back: %v", got)
	}
}
