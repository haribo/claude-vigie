package store

import (
	"context"
	"fmt"
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
