package tui

import (
	"testing"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestIsActive(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	cases := []struct {
		name string
		s    api.SessionView
		want bool
	}{
		{"recent working", api.SessionView{Status: "working", LastSeenAt: rfc(time.Minute)}, true},
		{"recent idle", api.SessionView{Status: "idle", LastSeenAt: rfc(30 * time.Minute)}, true},
		{"ended is hidden", api.SessionView{Status: "ended", LastSeenAt: rfc(time.Minute)}, false},
		{"stale is hidden", api.SessionView{Status: "idle", LastSeenAt: rfc(2 * time.Hour)}, false},
		{"unparseable is kept", api.SessionView{Status: "idle", LastSeenAt: "not-a-time"}, true},
	}
	for _, c := range cases {
		if got := isActive(c.s, now); got != c.want {
			t.Errorf("%s: isActive = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWatcherStale(t *testing.T) {
	cases := []struct {
		name string
		seen string
		want bool
	}{
		{"never seen", "", true},
		{"recent", time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339), false},
		{"old", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), true},
		{"unparseable is not stale", "not-a-time", false},
	}
	for _, c := range cases {
		m := model{watcherSeen: c.seen}
		if got := m.watcherStale(); got != c.want {
			t.Errorf("%s: watcherStale = %v, want %v", c.name, got, c.want)
		}
	}
}
