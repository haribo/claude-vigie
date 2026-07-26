package cli

import (
	"flag"
	"fmt"
	"os"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "address the server listens on")
	dbPath := fs.String("db", "claude-fleet.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fmt.Fprintf(os.Stderr, "serve: addr=%s db=%s\n", *addr, *dbPath)
	return notImplemented("serve")
}
