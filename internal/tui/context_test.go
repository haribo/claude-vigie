package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestContextWindow(t *testing.T) {
	cases := map[string]int64{
		"claude-opus-4-8":   1_000_000, // opus 4.6+ → 1M
		"claude-opus-4-6":   1_000_000,
		"claude-opus-4-5":   200_000, // below 4.6 → 200K
		"claude-sonnet-5":   1_000_000,
		"claude-sonnet-4-6": 1_000_000,
		"claude-haiku-4-5":  200_000, // haiku → 200K
		"claude-fable-5":    1_000_000,
		"":                  200_000, // unknown → conservative 200K
		"weird-model":       200_000,
	}
	for model, want := range cases {
		if got := contextWindow(model); got != want {
			t.Errorf("contextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}

func ctxPtr(v int64) *int64 { return &v }

func TestContextCell(t *testing.T) {
	// 100k of a 200k (haiku) window = 50%.
	if got := contextCell(api.SessionView{Model: "claude-haiku-4-5", ContextTokens: ctxPtr(100_000)}); got != "50%" {
		t.Errorf("contextCell = %q, want 50%%", got)
	}
	// 500k of a 1M (opus 4.8) window = 50%.
	if got := contextCell(api.SessionView{Model: "claude-opus-4-8", ContextTokens: ctxPtr(500_000)}); got != "50%" {
		t.Errorf("contextCell = %q, want 50%%", got)
	}
	// No reading at all (nil) → a dash, never a misleading 0%.
	if got := contextCell(api.SessionView{Model: "claude-opus-4-8"}); got != "-" {
		t.Errorf("contextCell(no tokens) = %q, want -", got)
	}
	// A known-empty context (a just-cleared session) → 0%, not a dash (#367).
	if got := contextCell(api.SessionView{Model: "claude-opus-4-8", ContextTokens: ctxPtr(0)}); got != "0%" {
		t.Errorf("contextCell(known 0) = %q, want 0%%", got)
	}
}
