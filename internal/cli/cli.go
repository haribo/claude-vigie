// Package cli implements the claude-fleet command-line dispatch.
//
// A single binary exposes several subcommands: `serve` (server + web
// dashboard), `tui` (terminal client), `report` (reporter invoked by Claude
// Code hooks), and `init` (install hooks + write the client config).
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/haribo/claude-fleet/internal/version"
)

// Run dispatches to the requested subcommand and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "serve":
		return runServe(rest)
	case "tui":
		return runTUI(rest)
	case "report":
		return runReport(rest)
	case "init":
		return runInit(rest)
	case "version", "--version", "-v":
		fmt.Println(version.String())
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
	fmt.Fprint(w, `claude-fleet — monitor Claude Code sessions across machines

Usage:
  claude-fleet <command> [flags]

Commands:
  serve      Run the fleet server and web dashboard
  tui        Run the terminal dashboard client
  report     Report a session event (invoked by Claude Code hooks)
  init       Install hooks and write the client config
  version    Print version information
  help       Print this help

Run "claude-fleet <command> -h" for command-specific flags.
`)
}

// notImplemented reports that a subcommand is not wired up yet. Bootstrap
// skeleton — each subcommand is implemented in its own tracked issue.
func notImplemented(cmd string) int {
	fmt.Fprintf(os.Stderr, "%s: not implemented yet\n", cmd)
	return 1
}
