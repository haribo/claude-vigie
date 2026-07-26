package main

import (
	"os"

	"github.com/haribo/claude-fleet/internal/daemon"
)

func main() {
	os.Exit(daemon.Run(os.Args[1:]))
}
