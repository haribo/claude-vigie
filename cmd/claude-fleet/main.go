package main

import (
	"os"

	"github.com/haribo/claude-fleet/internal/client"
)

func main() {
	os.Exit(client.Run(os.Args[1:]))
}
