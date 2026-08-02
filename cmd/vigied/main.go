package main

import (
	"os"

	"github.com/haribo/claude-vigie/internal/daemon"
)

func main() {
	os.Exit(daemon.Run(os.Args[1:]))
}
