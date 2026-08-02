package main

import (
	"os"

	"github.com/haribo/claude-vigie/internal/client"
)

func main() {
	os.Exit(client.Run(os.Args[1:]))
}
