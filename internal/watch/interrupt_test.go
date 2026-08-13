package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// TestRefineStatusInterrupted is the #351 refinement: an idle session whose last
// message is the interrupt marker keeps the base idle but shows DETAIL
// "interrupted"; a non-idle base is untouched, and the flag only overrides the
// activity when set.
func TestRefineStatusInterrupted(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no compaction markers
	now := time.Now()

	if base, act := refineStatus("idle", "Bash: build", "s", &transcript.Info{Interrupted: true}, time.Minute, now); base != "idle" || act != "interrupted" {
		t.Errorf("idle + interrupted = (%q, %q), want (idle, interrupted)", base, act)
	}
	if _, act := refineStatus("idle", "Read foo.go", "s", &transcript.Info{}, time.Minute, now); act != "Read foo.go" {
		t.Errorf("idle without interrupt must keep its activity, got %q", act)
	}
	if base, act := refineStatus("working", "editing", "s", &transcript.Info{Interrupted: true}, time.Minute, now); base != "working" || act == "interrupted" {
		t.Errorf("interrupt must only mark an idle base: (%q, %q)", base, act)
	}
}
