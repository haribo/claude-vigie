// Package daemon implements the vigied server command-line dispatch.
//
// The daemon runs on the host machine: it exposes the HTTP API, stores state
// in SQLite, streams updates over SSE, and serves the embedded web dashboard.
// Clients configure and report through the separate `vigie` binary.
package daemon

import (
	"fmt"
	"io"
	"os"

	"github.com/haribo/claude-vigie/internal/version"
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
	case "stats-repair":
		return runStatsRepair(rest)
	case "version", "--version", "-v":
		fmt.Println(version.String("vigied"))
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
	fmt.Fprint(w, `vigied — Claude Vigie server

Usage:
  vigied <command> [flags]

Commands:
  serve      Run the fleet server and web dashboard
  token      Print the fleet auth token (from the database)
  stats-repair  Correct one day's output-token figure in the analytics table
  version    Print version information
  help       Print this help

Run "vigied serve -h" for command-specific flags.
Clients configure and report via the separate "vigie" binary.
`)
}
