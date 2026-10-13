package watch

import "time"

// procStartFixture writes a start time the way Claude Code's registry does on
// macOS: a C-style date in UTC, to the second (ADR-0018). #869: read as ticks,
// it parsed to 0 and a dead session never ended.
func procStartFixture(start uint64) string {
	return time.Unix(int64(start), 0).UTC().Format(time.ANSIC) //nolint:gosec // test fixture values are small
}
