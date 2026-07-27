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

	samples, err := st.ListSamples(ctx, "s1", 100)
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
