package cli

import (
	"flag"
	"fmt"
	"os"
)

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	server := fs.String("server", "", "fleet server URL (defaults to the value in the client config)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fmt.Fprintf(os.Stderr, "tui: server=%q\n", *server)
	return notImplemented("tui")
}
