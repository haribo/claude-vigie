// Package client implements the vigie client command-line dispatch.
//
// The client is installed on every machine running Claude Code sessions. It
// configures reporting (`init`), reports session events invoked by hooks
// (`report`), and shows the live dashboard in the terminal (`tui`). The server
// runs separately as the `vigied` daemon.
package client

import (
	"fmt"
	"io"
	"os"

	"github.com/haribo/claude-vigie/internal/version"
)

// Run dispatches to the requested client subcommand and returns an exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return runInit(rest)
	case "hooks":
		return runHooks(rest)
	case "report":
		return runReport(rest)
	case "watch":
		return runWatch(rest)
	case "tui":
		return runTUI(rest)
	case "version", "--version", "-v":
		fmt.Println(version.String("vigie"))
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
	fmt.Fprint(w, `vigie — client for the Claude Vigie monitor

Usage:
  vigie <command> [flags]

Commands:
  init       Install hooks and write the client config
  hooks      Install/remove reporting hooks for one leg (VIGIE_CONFIG-selected)
  report     Report a session event (invoked by Claude Code hooks)
  watch      Watch local transcripts and report all sessions
  tui        Run the terminal dashboard client
  version    Print version information
  help       Print this help

Run "vigie <command> -h" for command-specific flags.
The server runs separately as "vigied".
`)
}
