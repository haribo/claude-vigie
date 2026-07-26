package main

import (
	"os"

	"github.com/haribo/claude-fleet/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
