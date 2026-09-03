package tui

import (
	"fmt"
	"time"
)

// humanizeDuration renders a duration compactly as a single unit: seconds,
// minutes, hours, or days (e.g. 3s, 12m, 1h, 2d). Shared by the relative
// last-seen column and any duration display.
func humanizeDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// relativeAge renders how long ago rfc (an RFC3339 timestamp) was, relative to
// now (e.g. "12m"). It returns "-" if rfc is empty or unparseable.
func relativeAge(rfc string, now time.Time) string {
	if rfc == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return "-"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return humanizeDuration(d)
}

// HumanizeDuration is humanizeDuration, exported for the client's preflight
// notice about refused reports (ADR-0013). An age is worded the same wherever the
// operator reads it; a second formatter would be one more rule to keep in step.
func HumanizeDuration(d time.Duration) string { return humanizeDuration(d) }
