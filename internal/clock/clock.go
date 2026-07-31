// Package clock centralizes access to the wall clock. It is the one sanctioned
// caller of time.Now(): every other package injects a func() time.Time and
// defaults it to clock.Now, so forbidigo can ban time.Now() everywhere else and
// business logic stays testable with a fake clock.
package clock

import "time"

// Now returns the current wall-clock time.
func Now() time.Time { return time.Now() }
