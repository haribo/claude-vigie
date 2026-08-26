package modelinfo

import "testing"

// TestWindow is the table that used to live in internal/tui/context_test.go and
// moved here with the code it covers (ADR-0011, #616). Same cases, same
// expectations — the point of the move is that there is now one of it.
func TestWindow(t *testing.T) {
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
		if got := Window(model); got != want {
			t.Errorf("Window(%q) = %d, want %d", model, got, want)
		}
	}
}

// A non-numeric version part must read as 0 rather than be coerced: "opus-4x"
// is not opus 4, and treating it as such would hand it the wrong window.
func TestANonNumericVersionIsNotCoerced(t *testing.T) {
	if got := Window("claude-opus-4x-9"); got != 200_000 {
		t.Errorf("Window(opus-4x-9) = %d, want the conservative 200K", got)
	}
}
