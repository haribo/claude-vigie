// Package status holds the one list of session statuses vigie can report.
//
// The vocabulary itself is specified in docs/design/session-status.md § 1; this
// package is its executable copy, and status_test.go fails if the two disagree.
// The design document stays the source of truth — the code is checked against it,
// never the other way round.
//
// It exists because the list used to be hand-copied into four places (the metrics
// gauge, the TUI sort, the web dashboard, the GNOME extension), each incomplete in
// a different way and none of them checked. Adding `compacting` (#342) reached two
// of the four, and nothing noticed for two releases (#421, #422, #423).
package status

// All is every status a session can hold, in the order the design document lists
// them: roughly most-active first, ending with the two that mean "not running".
//
// Order matters to consumers that group or pre-declare series, so it is part of
// the contract rather than an accident of declaration.
var All = []string{
	"working",
	"thinking",
	"compacting",
	"waiting",
	"stalled",
	"idle",
	"error",
	"stale",
	"ended",
}

// Known reports whether s is a status vigie can produce. A consumer that renders
// unknown statuses defensively (rather than dropping them) is doing the right
// thing; this is for the ones that must decide, such as styling.
func Known(s string) bool {
	for _, k := range All {
		if k == s {
			return true
		}
	}
	return false
}

// Order is every status ranked for the status sort, most active first, exactly as
// docs/design/session-list.md § 2.1 lists them.
//
// It is separate from All because the two answer different questions: All is what
// exists, Order is how it sorts. Keeping the sort partial was the defect in #464 —
// the four statuses nobody ranked fell to a default that put them below `ended` in
// the TUI, and produced a NaN comparator in the web dashboard, which stops sorting
// altogether rather than sorting badly.
var Order = []string{
	"stalled",
	"working",
	"thinking",
	"compacting",
	"waiting",
	"idle",
	"error",
	"stale",
	"ended",
}

// Rank is a status's position in the sort, lower first. An unknown status sorts
// last rather than first: a status this build has never heard of is the one thing
// we can say least about, so it does not get to head the table.
func Rank(s string) int {
	for i, k := range Order {
		if k == s {
			return i
		}
	}
	return len(Order)
}
