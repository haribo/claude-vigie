package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestHumanizeTokens(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1500: "1.5k", 2_500_000: "2.5M"}
	for n, want := range cases {
		if got := humanizeTokens(n); got != want {
			t.Errorf("humanizeTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestShortModel(t *testing.T) {
	if got := shortModel("claude-opus-4-8"); got != "opus-4-8" {
		t.Errorf("shortModel = %q, want opus-4-8", got)
	}
}

func TestClockTime(t *testing.T) {
	if got := clockTime("2026-07-26T17:01:32Z"); got != "17:01:32" {
		t.Errorf("clockTime = %q, want 17:01:32", got)
	}
	if got := clockTime("weird"); got != "weird" {
		t.Errorf("clockTime fallback = %q, want weird", got)
	}
}

func TestPadAndTruncate(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q, want %q", got, "ab   ")
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q, want %q", got, "abc…")
	}
}

func TestRenderTableContainsData(t *testing.T) {
	out := renderTable([]api.SessionView{{
		ID: "s1", Machine: "laptop", ProjectDir: "/home/x/proj", GitBranch: "main",
		Model: "claude-opus-4-8", Status: "working",
		Usage:      api.Usage{OutputTokens: 1500, InputTokens: 500},
		LastSeenAt: "2026-07-26T17:01:32Z",
	}})
	for _, want := range []string{"STATUS", "laptop", "proj", "main", "opus-4-8", "1.5k", "17:01:32"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered table missing %q:\n%s", want, out)
		}
	}
}
