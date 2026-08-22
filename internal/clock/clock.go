// Package clock centralizes access to the wall clock. It is the one sanctioned
// caller of time.Now(), time.Since() and time.Until(): every other package
// injects a func() time.Time and defaults it to clock.Now, so forbidigo can ban
// all three everywhere else and business logic stays testable with a fake clock.
package clock

import "time"

// Now returns the current wall-clock time.
func Now() time.Time { return time.Now() }

// Since is the elapsed time since t, and Until the time remaining before it.
//
// They exist so forbidigo can ban time.Since and time.Until alongside time.Now:
// the three read the same hidden global, and banning one name while leaving its
// two aliases open is a rule that enforces spelling rather than the property it
// wants (#601). Business logic still injects a func() time.Time and subtracts;
// these are for the presentation edge, where clock.Now is already the sanctioned
// call.
func Since(t time.Time) time.Duration { return Now().Sub(t) }

// Until is the time remaining before t. See Since.
func Until(t time.Time) time.Duration { return t.Sub(Now()) }
