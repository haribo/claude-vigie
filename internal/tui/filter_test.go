package tui

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestPrefsVisible(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	// Defaults: hide ended, never hide idle by age.
	def := defaultPrefs()
	if !def.visible(api.SessionView{Status: "idle", LastSeenAt: rfc(3 * time.Hour)}, now) {
		t.Error("default: an old idle session should stay visible")
	}
	if def.visible(api.SessionView{Status: "ended", LastSeenAt: rfc(time.Minute)}, now) {
		t.Error("default: ended should be hidden")
	}

	// With idle_hide_after set, stale sessions are hidden; recent ones stay.
	p := prefs{hideEnded: true, idleHideAfter: time.Hour}
	if p.visible(api.SessionView{Status: "idle", LastSeenAt: rfc(2 * time.Hour)}, now) {
		t.Error("idleHideAfter: a 2h-old idle session should be hidden")
	}
	if !p.visible(api.SessionView{Status: "idle", LastSeenAt: rfc(30 * time.Minute)}, now) {
		t.Error("idleHideAfter: a 30m-old idle session should be visible")
	}
	if !p.visible(api.SessionView{Status: "idle", LastSeenAt: "not-a-time"}, now) {
		t.Error("unparseable timestamp should be kept")
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
		{"unparseable is stale", "not-a-time", true},
	}
	for _, c := range cases {
		m := model{gotWatcher: true, watcherMachines: map[string]string{"orion": c.seen}}
		// Through watcherRow, which is what actually renders: watcherStale() was a
		// second entry point to the same verdict and was left with no caller when the
		// banner went (#650), so a guard on it would have stopped covering anything.
		got := m.watcherRow().level == levelBroken
		if got != c.want {
			t.Errorf("%s: the watcher row reads broken = %v, want %v", c.name, got, c.want)
		}
	}
}
