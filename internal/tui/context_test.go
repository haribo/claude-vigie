package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func ctxPtr(v int64) *int64 { return &v }

func pctPtr(v int) *int { return &v }

// TestContextCell now asks the cell what it renders from a reading the daemon
// already derived (ADR-0011, #616). The model no longer enters into it: which
// window a percentage came from is settled before the view is built, and asking
// this layer about it was asking it to own a table it no longer holds.
func TestContextCell(t *testing.T) {
	if got := contextCell(api.SessionView{ContextTokens: ctxPtr(100_000), ContextPct: pctPtr(50)}); got != "50%" {
		t.Errorf("contextCell = %q, want 50%%", got)
	}
	// No reading at all (nil) → a dash, never a misleading 0%.
	if got := contextCell(api.SessionView{Model: "claude-opus-4-8"}); got != "-" {
		t.Errorf("contextCell(no tokens) = %q, want -", got)
	}
	// A known-empty context (a just-cleared session) → 0%, not a dash (#367).
	if got := contextCell(api.SessionView{ContextTokens: ctxPtr(0), ContextPct: pctPtr(0)}); got != "0%" {
		t.Errorf("contextCell(known 0) = %q, want 0%%", got)
	}
	// Half an invariant is not the invariant: a payload carrying one pointer and
	// not the other must render a dash, not panic mid-draw.
	if got := contextCell(api.SessionView{ContextTokens: ctxPtr(100_000)}); got != "-" {
		t.Errorf("contextCell(tokens without pct) = %q, want -", got)
	}
}
