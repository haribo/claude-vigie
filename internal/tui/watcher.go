package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// watcherStaleAfter is how long the server may go without a watch report before
// the TUI warns that statuses may be stale.
const watcherStaleAfter = 15 * time.Second

// watcherVerdict is what a recorded heartbeat timestamp says about whether the
// statuses on screen can be trusted. Three outcomes, not two: a timestamp that
// cannot be read is neither a healthy watcher nor a silent one, and collapsing it
// into either loses the only thing that tells the operator where to look
// (docs/design/watcher-liveness.md § 5).
type watcherVerdict int

const (
	watcherReporting  watcherVerdict = iota // beat within watcherStaleAfter
	watcherSilent                           // no heartbeat recorded, or an old one
	watcherUnreadable                       // a heartbeat recorded, and unparseable
)

// alarm reports whether this verdict means the statuses on screen may be frozen.
// Unreadable is one: the indicator answers "can the operator trust what is on
// screen", and an answer that cannot be read answers that with no.
func (v watcherVerdict) alarm() bool { return v != watcherReporting }

// readWatcher is the single rule for turning a recorded heartbeat into a verdict.
//
// It is one function because it used to be two, in the same package, disagreeing:
// the state pill read an unparseable timestamp as healthy ("don't cry wolf") while
// the Machines tab read the same shape as a missing watcher, so one screen could
// show `watcher · reporting` beside `⚠ none` for the same machine. #284 settled
// this rule once; a second implementation is how it came undone (#600).
//
// Every indicator is a caller. Adding one means calling this, never re-deriving
// it — that is the whole point of the file.
func readWatcher(seen string, now time.Time) watcherVerdict {
	if seen == "" {
		return watcherSilent
	}
	t, err := time.Parse(time.RFC3339, seen)
	if err != nil {
		return watcherUnreadable
	}
	if now.Sub(t) > watcherStaleAfter {
		return watcherSilent
	}
	return watcherReporting
}

// fleetWatchers is the whole-fleet reading: how many machines are known, and
// which of them beat and then stopped — split by cause, because the two send the
// operator to different places (§ 5). Both lists are sorted, so the alarm text is
// stable across renders.
//
// The count and the names come from the per-machine map rather than the daemon's
// global `watch_seen`, which every heartbeat from every machine overwrites — one
// live watcher there hides every dead one, and the more machines a fleet has the
// more reliably it hides them (docs/design/watcher-liveness.md § 6, #599).
//
// A machine with no recorded heartbeat is reporting through hooks alone, which is
// a deployment choice and not a fault: counted as known, never listed.
func fleetWatchers(machines map[string]string, now time.Time) (known int, silent, unreadable []string) {
	for name, seen := range machines {
		known++
		if seen == "" {
			continue // never beat: hooks-only, by choice
		}
		switch readWatcher(seen, now) {
		case watcherSilent:
			silent = append(silent, name)
		case watcherUnreadable:
			unreadable = append(unreadable, name)
		}
	}
	sort.Strings(silent)
	sort.Strings(unreadable)
	return known, silent, unreadable
}

// fleetAlarm reports whether the statuses on screen may be frozen anywhere in the
// fleet, and names the machines responsible.
//
// It is true with no names in one case: a fleet where nothing beats at all — no
// watcher was ever started — where no individual machine qualifies as having
// stopped, yet nothing is refreshing anything. That is precisely the situation
// the indicator exists for, so it stays an alarm (§ 6).
func fleetAlarm(machines map[string]string, now time.Time) (alarm bool, known int, silent, unreadable []string) {
	known, silent, unreadable = fleetWatchers(machines, now)
	if len(silent) > 0 || len(unreadable) > 0 {
		return true, known, silent, unreadable
	}
	for _, seen := range machines {
		if readWatcher(seen, now) == watcherReporting {
			return false, known, nil, nil
		}
	}
	return true, known, nil, nil
}

// fleetAlarmDetail is the state row's text: how much of the fleet is affected and
// which machines, so the operator does not have to open the Machines tab to learn
// where to go (§ 6).
func fleetAlarmDetail(known int, silent, unreadable []string) string {
	names := append(append([]string{}, silent...), unreadable...)
	if len(names) == 0 {
		return "not reporting · statuses may be frozen"
	}
	sort.Strings(names)
	what := "not reporting"
	if len(silent) == 0 {
		what = "unreadable heartbeat" // every cause is a heartbeat we cannot read
	}
	return fmt.Sprintf("%d of %d %s (%s)", len(names), known, what, strings.Join(names, ", "))
}
