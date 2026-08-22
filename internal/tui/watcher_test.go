package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestReadWatcherVerdicts covers the three outcomes of
// docs/design/watcher-liveness.md § 5. The third is the one #600 settled: a
// heartbeat that cannot be read is its own verdict, neither healthy nor merely
// absent, because the two send the operator to different places.
func TestReadWatcherVerdicts(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		seen string
		want watcherVerdict
	}{
		{"a recent heartbeat", now.Add(-5 * time.Second).Format(time.RFC3339), watcherReporting},
		{"one on the threshold", now.Add(-watcherStaleAfter).Format(time.RFC3339), watcherReporting},
		{"an old one", now.Add(-time.Minute).Format(time.RFC3339), watcherSilent},
		{"none at all", "", watcherSilent},
		{"one that will not parse", "not-a-time", watcherUnreadable},
	}
	for _, c := range cases {
		if got := readWatcher(c.seen, now); got != c.want {
			t.Errorf("%s: readWatcher = %v, want %v", c.name, got, c.want)
		}
	}
	if !watcherUnreadable.alarm() || !watcherSilent.alarm() || watcherReporting.alarm() {
		t.Error("alarm() must hold for both failures and only for them")
	}
}

// TestBothIndicatorsAgreeOnAnUnreadableHeartbeat is the regression test for #600.
//
// The state pill used to read an unparseable timestamp as healthy ("don't cry
// wolf") while the Machines tab read the same shape as a missing watcher, so one
// screen showed `watcher · reporting` at the top and `⚠ none` in the tab, for the
// same machine at the same moment. Both now derive from readWatcher, so the only
// way to make them disagree again is to stop calling it.
//
// The fixture is unreadable rather than old on purpose: that verdict does not
// depend on the clock, so the two sides are compared on identical input without
// either one's time source entering into it.
func TestBothIndicatorsAgreeOnAnUnreadableHeartbeat(t *testing.T) {
	const seen = "not-a-time"

	m := model{clock: fixedClock, gotWatcher: true, watcherSeen: seen}
	row := rowsByLabel(m)["watcher"]
	if row.level != levelBroken {
		t.Errorf("state pill level = %v, want red — an unreadable heartbeat means the screen may be lying", row.level)
	}
	if !strings.Contains(row.detail, "unreadable") {
		t.Errorf("state pill detail = %q, want it to name the unreadable heartbeat rather than blame the machine", row.detail)
	}

	out := renderMachines(
		[]api.SessionView{{ID: "s1", Machine: "orion", Status: "working"}},
		map[string]string{"orion": seen},
		map[string]api.VersionInfo{}, 300)
	if strings.Contains(out, "● live") {
		t.Error("the Machines tab shows a live watcher while the state pill calls it broken — the two must not disagree (#600)")
	}
	if !strings.Contains(out, "⚠ time?") {
		t.Errorf("the Machines tab should name the unreadable heartbeat, got:\n%s", out)
	}
}
