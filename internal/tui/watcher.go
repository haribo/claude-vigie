package tui

import "time"

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
