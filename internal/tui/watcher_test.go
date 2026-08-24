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

	m := model{clock: fixedClock, gotWatcher: true, watcherMachines: map[string]string{"orion": seen}}
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
		map[string]api.VersionInfo{}, 300, fixedClock())
	if strings.Contains(out, "● live") {
		t.Error("the Machines tab shows a live watcher while the state pill calls it broken — the two must not disagree (#600)")
	}
	if !strings.Contains(out, "⚠ time?") {
		t.Errorf("the Machines tab should name the unreadable heartbeat, got:\n%s", out)
	}
}

// TestBothIndicatorsAgreeOnTheClockDependentVerdicts is the case #600 could not
// assert and #609 unlocked.
//
// "Unreadable" is clock-independent, so it was the only verdict the two
// indicators could be compared on while the Machines tab read the package clock
// and the state pill read the model's. These two are the ordinary ones — a
// heartbeat that is recent, and one that has gone silent — and they are only
// decidable against a clock both sides share.
//
// The fixtures are dated from fixedClock, which is weeks off the wall clock. That
// is deliberate: read against time.Now() the "recent" heartbeat is ancient, so the
// tab would show a missing watcher next to a pill calling it healthy — the very
// disagreement #600 removed, reintroduced through the clock instead of the rule.
func TestBothIndicatorsAgreeOnTheClockDependentVerdicts(t *testing.T) {
	now := fixedClock()
	cases := []struct {
		name      string
		seen      string
		wantLevel level
		wantRow   string
		wantCell  string
	}{
		{"a recent heartbeat", now.Add(-5 * time.Second).UTC().Format(time.RFC3339), levelOK, "reporting", "● live"},
		{"a silent watcher", now.Add(-time.Minute).UTC().Format(time.RFC3339), levelBroken, "not reporting", "⚠ none"},
	}
	for _, c := range cases {
		m := model{clock: fixedClock, gotWatcher: true, watcherMachines: map[string]string{"orion": c.seen}}
		row := rowsByLabel(m)["watcher"]
		if row.level != c.wantLevel {
			t.Errorf("%s: state pill level = %v, want %v", c.name, row.level, c.wantLevel)
		}
		if !strings.Contains(row.detail, c.wantRow) {
			t.Errorf("%s: state pill detail = %q, want it to contain %q", c.name, row.detail, c.wantRow)
		}

		out := renderMachines(
			[]api.SessionView{{ID: "s1", Machine: "orion", Status: "working"}},
			map[string]string{"orion": c.seen},
			map[string]api.VersionInfo{}, 300, now)
		if !strings.Contains(out, c.wantCell) {
			t.Errorf("%s: the Machines tab should show %q while the pill shows %q, got:\n%s",
				c.name, c.wantCell, row.detail, out)
		}
	}
}

// TestTheFleetVerdictCoversEveryMachine is the regression test for #599.
//
// The state pill used to read the daemon's global `watch_seen` — the most recent
// beat from anywhere — so one live watcher hid every dead one: on three machines,
// orion going silent left the pill green while orion's sessions sat frozen, and
// the only place saying so was a tab the operator had to already suspect.
//
// The three cases are the ones docs/design/watcher-liveness.md § 6 settles: a
// machine that stopped is an alarm and is named, a machine that never beat is a
// deployment choice and is not, and a fleet with nothing beating at all is an
// alarm even though no single machine stopped.
func TestTheFleetVerdictCoversEveryMachine(t *testing.T) {
	now := fixedClock()
	live := now.Add(-2 * time.Second).UTC().Format(time.RFC3339)
	dead := now.Add(-time.Minute).UTC().Format(time.RFC3339)

	cases := []struct {
		name      string
		machines  map[string]string
		wantLevel level
		wantText  string
	}{
		{
			name:      "one of three stopped is an alarm that names it",
			machines:  map[string]string{"orion": dead, "box": live, "nova": live},
			wantLevel: levelBroken,
			wantText:  "1 of 3 not reporting (orion)",
		},
		{
			name:      "two stopped are both named, in a stable order",
			machines:  map[string]string{"orion": dead, "box": dead, "nova": live},
			wantLevel: levelBroken,
			wantText:  "2 of 3 not reporting (box, orion)",
		},
		{
			// The case option 1 would have got wrong: a machine reporting on hooks
			// alone is a choice, and making it red forever is how an indicator stops
			// being read.
			name:      "a hooks-only machine beside a live one is not an alarm",
			machines:  map[string]string{"hooks-only": "", "box": live},
			wantLevel: levelOK,
			wantText:  "reporting",
		},
		{
			// …and the exception to that: nobody is refreshing anything.
			name:      "no watcher anywhere is still an alarm",
			machines:  map[string]string{"hooks-only": "", "other": ""},
			wantLevel: levelBroken,
			wantText:  "not reporting · statuses may be frozen",
		},
		{
			name:      "an unreadable heartbeat names its own cause",
			machines:  map[string]string{"orion": "not-a-time", "box": live},
			wantLevel: levelBroken,
			wantText:  "1 of 2 unreadable heartbeat (orion)",
		},
	}
	for _, c := range cases {
		m := model{clock: fixedClock, gotWatcher: true, watcherMachines: c.machines}
		row := rowsByLabel(m)["watcher"]
		if row.level != c.wantLevel {
			t.Errorf("%s: level = %v, want %v", c.name, row.level, c.wantLevel)
		}
		if row.detail != c.wantText {
			t.Errorf("%s: detail = %q, want %q", c.name, row.detail, c.wantText)
		}
	}
}
