package tui

import (
	"testing"
	"time"
)

// The watcher verdict is one of the few rules ADR-0011 leaves duplicated on
// purpose: it is a function of *now*, so it decays between fetches and has to be
// derived where it is displayed (#617). Duplicated therefore, but not unchecked —
// this and test/js/dashboard.test.mjs read the same case list and must agree case
// for case, including the exact alarm text (#623).
type watcherFixture struct {
	Verdict []struct {
		Why  string `json:"why"`
		Seen string `json:"seen"`
		Now  string `json:"now"`
		Want string `json:"want"`
	} `json:"verdict"`
	Fleet []struct {
		Why      string            `json:"why"`
		Machines map[string]string `json:"machines"`
		Now      string            `json:"now"`
		Alarm    bool              `json:"alarm"`
		Detail   string            `json:"detail"`
	} `json:"fleet"`
}

func loadWatcherFixture(t *testing.T) watcherFixture {
	t.Helper()
	f := loadFixture[watcherFixture](t, "watcher-cases.json")
	if len(f.Verdict) == 0 || len(f.Fleet) == 0 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	return f
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("fixture carries an unparseable `now` %q: %v", s, err)
	}
	return v
}

var verdictNames = map[watcherVerdict]string{
	watcherReporting:  "reporting",
	watcherSilent:     "silent",
	watcherUnreadable: "unreadable",
}

func TestReadWatcherAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadWatcherFixture(t).Verdict {
		if got := verdictNames[readWatcher(c.Seen, mustTime(t, c.Now))]; got != c.Want {
			t.Errorf("readWatcher(%q) = %s, want %s — %s", c.Seen, got, c.Want, c.Why)
		}
	}
}

func TestFleetAlarmAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadWatcherFixture(t).Fleet {
		alarm, known, silent, unreadable := fleetAlarm(c.Machines, mustTime(t, c.Now))
		if alarm != c.Alarm {
			t.Errorf("fleetAlarm(%v) = %v, want %v — %s", c.Machines, alarm, c.Alarm, c.Why)
			continue
		}
		// "reporting" is the state row's wording for no alarm; the detail builder is
		// only reached when there is one.
		got := "reporting"
		if alarm {
			got = fleetAlarmDetail(known, silent, unreadable)
		}
		if got != c.Detail {
			t.Errorf("detail = %q, want %q — %s", got, c.Detail, c.Why)
		}
	}
}
