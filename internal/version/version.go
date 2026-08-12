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

// Match reports whether two builds are compatible: strict equality of the version
// string when both sides are a real release, and commit equality when either side
// is a dev build — a "dev" == "dev" string match across two different commits is a
// false pass (#357).
//
// This is the single implementation of the fleet's version rule, shared by the TUI
// preflight and the daemon's watcher gate; two copies of a consistency rule drift
// (docs/design/version-consistency.md, #384).
func Match(aVersion, aCommit, bVersion, bCommit string) bool {
	if aVersion == "dev" || bVersion == "dev" {
		return aCommit == bCommit
	}
	return aVersion == bVersion
}

// Describe renders a build for an operator-facing message: the version, plus the
// commit when it carries information.
func Describe(v, commit string) string {
	if commit != "" && commit != "none" {
		return fmt.Sprintf("%s (commit %s)", v, commit)
	}
	if v == "" {
		return "an undeclared build"
	}
	return v
}
