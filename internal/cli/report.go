package cli

import (
	"flag"
	"fmt"
	"os"
)

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	event := fs.String("event", "", "hook event name (SessionStart, Stop, PostToolUse, SessionEnd, ...)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *event == "" {
		fmt.Fprintln(os.Stderr, "report: --event is required")
		return 2
	}

	fmt.Fprintf(os.Stderr, "report: event=%s\n", *event)
	return notImplemented("report")
}
