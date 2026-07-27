package tui

import "testing"

func TestSparklineBraille(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
	// Two samples per glyph: [max, max] fills both columns → ⣿ (U+28FF).
	if got := sparkline([]int{4, 4}); got != "⣿" {
		t.Errorf("full pair = %q, want ⣿", got)
	}
	// A small pair relative to the max fills only the bottom row → ⣀ (U+28C0).
	if got := []rune(sparkline([]int{1, 1, 4})); string(got[0]) != "⣀" {
		t.Errorf("low pair = %q, want ⣀ (bottom row)", string(got[0]))
	}
	// Odd length: the last sample fills only the left column of a final glyph.
	if got := []rune(sparkline([]int{4, 4, 4})); len(got) != 2 {
		t.Errorf("3 samples => %d glyphs, want 2", len(got))
	}
}

func TestSparkHeight(t *testing.T) {
	if h := sparkHeight(10, 10); h != 4 {
		t.Errorf("max maps to %d, want 4", h)
	}
	if h := sparkHeight(0, 10); h != 0 {
		t.Errorf("zero maps to %d, want 0", h)
	}
}
