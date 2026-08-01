package client

import (
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/report"
)

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	event := fs.String("event", "", "hook event name (SessionStart, Stop, SessionEnd, ...)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Fire-and-forget: a monitoring hook must never fail a Claude session.
	// Log any problem to stderr (captured in the hook debug log) and exit 0.
	if err := report.Run(*event, os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "vigie report: %v\n", err)
	}
	return 0
}
