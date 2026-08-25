// Package modelinfo holds what vigie derives from a Claude model's name —
// today, the size of its context window.
//
// It is shared rather than client-side because the daemon computes the context
// fill for every client (ADR-0011): the table below is maintained by hand, and
// it used to be maintained by hand twice, once in Go for the TUI and once in
// JavaScript for the dashboard. A new model had to be taught to two files in two
// languages, and the second one was the one that got forgotten.
package modelinfo

import (
	"strconv"
	"strings"
)

// Context window sizes. A wrong guess only skews a percentage, never a status
// (#279), which is why an unrecognized model gets the conservative one.
const (
	bigWindow  = 1_000_000
	baseWindow = 200_000
)

// Window returns a model's context window in tokens. Opus and Sonnet at 4.6+
// carry 1M, as does Fable 5 and anything above; Haiku, older models and anything
// unrecognized carry 200K. Maintained by hand.
func Window(model string) int64 {
	family, major, minor := version(short(model))
	switch family {
	case "fable":
		return bigWindow
	case "opus", "sonnet":
		if major > 4 || (major == 4 && minor >= 6) {
			return bigWindow
		}
		return baseWindow
	default: // haiku, unknown, empty
		return baseWindow
	}
}

// short drops the vendor prefix: "claude-opus-4-8" → "opus-4-8".
func short(m string) string { return strings.TrimPrefix(m, "claude-") }

// version splits a short model name ("opus-4-8", "sonnet-5") into its family and
// major/minor version; missing or non-numeric parts are 0.
func version(s string) (family string, major, minor int) {
	parts := strings.Split(s, "-")
	family = parts[0]
	if len(parts) > 1 {
		major, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		minor, _ = strconv.Atoi(parts[2])
	}
	return family, major, minor
}
