package client

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/haribo/claude-vigie/internal/report"
)

// runCall raises a call for the operator on the session this process runs in
// (ADR-0010). Everything after the flags is the message, so it needs no quoting
// discipline beyond the shell's own.
//
// Fire-and-forget, like the hooks: any problem is written to stderr and the exit
// code stays 0. A session must never fail because a monitoring signal could not
// be delivered.
func runCall(args []string) int {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: vigie call [message]

Raise a call for the operator on the current Claude Code session: vigie shows it
until work resumes in that session. The message is optional.

  vigie call "backfill done — 12k rows, 0 err"
`)
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if err := report.Call(strings.Join(fs.Args(), " ")); err != nil {
		fmt.Fprintf(os.Stderr, "vigie call: %v\n", err)
	}
	return 0
}
