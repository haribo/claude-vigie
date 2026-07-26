// Package version holds build metadata injected at link time via -ldflags.
package version

import "fmt"

// Build metadata. Overridden at build time by the justfile / CI ldflags.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("claude-fleet %s (commit %s, built %s)", Version, Commit, BuildTime)
}
