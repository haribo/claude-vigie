package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestPlatformDisplayWords(t *testing.T) {
	cases := []struct{ ind, word string }{
		{"none", "operational"},
		{"minor", "degraded"},
		{"major", "major outage"},
		{"critical", "critical outage"},
		{"", "unknown"},
		{"weird", "unknown"},
	}
	for _, c := range cases {
		if _, w, _ := platformDisplay(api.PlatformStatus{Indicator: c.ind}); w != c.word {
			t.Errorf("indicator %q → %q, want %q", c.ind, w, c.word)
		}
	}
}

func TestPlatformStrip(t *testing.T) {
	// No usable status: render nothing (degrades against a server without the poll).
	for _, ind := range []string{"", "weird"} {
		if s := platformStrip(api.PlatformStatus{Indicator: ind}); s != "" {
			t.Errorf("indicator %q should render nothing, got %q", ind, s)
		}
	}
	// major/critical keep a usable indicator (red) on the strip.
	s := platformStrip(api.PlatformStatus{Indicator: "none"})
	if !strings.Contains(s, "platform") || !strings.Contains(s, "operational") {
		t.Errorf("strip = %q, want platform/operational", s)
	}
	if s := platformStrip(api.PlatformStatus{Indicator: "major"}); !strings.Contains(s, "major outage") {
		t.Errorf("strip = %q, want major outage", s)
	}
}
