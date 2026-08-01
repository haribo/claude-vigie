package client

import (
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/tui"
)

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	server := fs.String("server", "", "fleet server URL (overrides the client config)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\nrun 'vigie init' first, or pass --server\n", err)
		return 1
	}
	if *server != "" {
		cfg.ServerURL = *server
	}

	if err := tui.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}
