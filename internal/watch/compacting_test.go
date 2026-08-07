package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/compaction"
	"github.com/haribo/claude-vigie/internal/transcript"
)

func TestWithCompacting(t *testing.T) {
	cases := []struct {
		base       string
		compacting bool
		want       string
	}{
		{"working", true, "compacting"},
		{"thinking", true, "compacting"},
		{"idle", true, "idle"},       // not a live turn — untouched
		{"waiting", true, "waiting"}, // needs a human — never overridden
		{"ended", true, "ended"},
		{"working", false, "working"},
	}
	for _, c := range cases {
		if got := withCompacting(c.base, c.compacting); got != c.want {
			t.Errorf("withCompacting(%q, %v) = %q, want %q", c.base, c.compacting, got, c.want)
		}
	}
}

// TestCompactingNow covers the #342 marker lifecycle: a fresh PreCompact marker
// opens compacting; a transcript boundary or the expiry closes it and sweeps the
// marker so the state self-heals.
func TestCompactingNow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }

	// No marker → not compacting.
	if compactingNow("none", &transcript.Info{}, now) {
		t.Error("no marker should not read as compacting")
	}

	// Fresh marker, no boundary → compacting.
	mustSave(t, "live", compaction.Marker{Started: rfc(-30 * time.Second), Trigger: "auto"})
	if !compactingNow("live", &transcript.Info{}, now) {
		t.Error("a fresh marker with no boundary should read as compacting")
	}

	// Boundary at/after the start → closed, and the marker is swept.
	mustSave(t, "closed", compaction.Marker{Started: rfc(-30 * time.Second)})
	if compactingNow("closed", &transcript.Info{LastCompactBoundary: rfc(-5 * time.Second)}, now) {
		t.Error("a boundary after the start should close compacting")
	}
	if _, ok, _ := compaction.Load("closed"); ok {
		t.Error("a closed marker should be swept")
	}

	// Older than the window → expired, swept.
	mustSave(t, "stale", compaction.Marker{Started: rfc(-2 * compactWindow)})
	if compactingNow("stale", &transcript.Info{}, now) {
		t.Error("a marker past the window should expire")
	}
	if _, ok, _ := compaction.Load("stale"); ok {
		t.Error("an expired marker should be swept")
	}
}

func mustSave(t *testing.T, id string, m compaction.Marker) {
	t.Helper()
	if err := compaction.Save(id, m); err != nil {
		t.Fatal(err)
	}
}
