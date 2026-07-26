package cli

import (
	"flag"
	"fmt"
	"os"
)

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	server := fs.String("server", "", "fleet server URL to report to")
	token := fs.String("token", "", "shared auth token")
	machine := fs.String("machine", "", "machine name (defaults to the hostname)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fmt.Fprintf(os.Stderr, "init: server=%q machine=%q token=%v\n", *server, *machine, *token != "")
	return notImplemented("init")
}
