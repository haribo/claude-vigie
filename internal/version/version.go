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
// String renders the version line for the given binary name (each binary passes
// its own, so `vigied version` does not print "vigie").
func String(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, Version, Commit, BuildTime)
}
