//go:build !darwin

package watch

import "strconv"

// procStartFixture writes a start time the way Claude Code's registry does on this
// platform: on Linux, /proc clock ticks as a decimal string.
func procStartFixture(start uint64) string { return strconv.FormatUint(start, 10) }
