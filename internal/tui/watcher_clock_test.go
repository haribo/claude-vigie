package tui

import (
	"testing"
	"time"
)

// TestWatcherStaleReadsTheModelClock pins the verdict to the model's injected
// clock rather than the wall clock (#601).
//
// Both fixtures are anchored to fixedClock, not to now, so against the wall
// clock they are weeks old and far past watcherStaleAfter: a watcherStale()
// reading time.Since() calls both of them stale and the "recent" case fails.
// That gap is what lets this test see the difference at all — and it is exactly
// what the pre-#601 tests could not do, having built their fixtures from
// time.Now() and so agreed with the wall clock by construction.
func TestWatcherStaleReadsTheModelClock(t *testing.T) {
	now := fixedClock()
	cases := []struct {
		name string
		seen time.Time
		want bool
	}{
		{"just before the injected now", now.Add(-2 * time.Second), false},
		{"well before the injected now", now.Add(-time.Minute), true},
	}
	for _, c := range cases {
		m := model{clock: fixedClock, watcherMachines: map[string]string{"orion": c.seen.UTC().Format(time.RFC3339)}}
		if got := m.watcherStale(); got != c.want {
			t.Errorf("%s: watcherStale = %v, want %v", c.name, got, c.want)
		}
	}
}
