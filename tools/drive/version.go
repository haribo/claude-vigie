//go:build linux

package main

import (
	"os/exec"
	"strings"
)

// claudeVersion asks the binary what it is, so the table carries the version it
// describes. A measurement of an undocumented private format is only true of the
// version it was taken on (ADR-0016).
func claudeVersion() string {
	out, err := exec.Command("claude", "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
