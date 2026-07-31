// Package daemon implements the claude-fleetd server command-line dispatch.
//
// The daemon runs on the host machine: it exposes the HTTP API, stores state
// in SQLite, streams updates over SSE, and serves the embedded web dashboard.
// Clients configure and report through the separate `claude-fleet` binary.
package daemon

import (
	"fmt"
	"io"
	"os"

	"github.com/haribo/claude-fleet/internal/version"
)

// Run dispatches to the requested daemon subcommand and returns an exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "serve":
		return runServe(rest)
	case "token":
		return runToken(rest)
	case "version", "--version", "-v":
		fmt.Println(version.String("claude-fleetd"))
		return 0
	case "help", "--help", "-h":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage(os.Stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `claude-fleetd — Claude Fleet server

Usage:
  claude-fleetd <command> [flags]

Commands:
  serve      Run the fleet server and web dashboard
  token      Print the fleet auth token (from the database)
  version    Print version information
  help       Print this help

Run "claude-fleetd serve -h" for command-specific flags.
Clients configure and report via the separate "claude-fleet" binary.
`)
}
