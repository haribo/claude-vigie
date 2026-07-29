package client

import (
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-fleet/internal/install"
)

// runHooks installs or removes one reporting leg. The leg is selected by
// FLEET_CONFIG: unset is the production leg; a path is a dev leg that reports to
// that config's server alongside production. Legs are independent.
func runHooks(args []string) int {
	if len(args) == 0 {
		hooksUsage()
		return 2
	}
	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("hooks", flag.ContinueOnError)
	detailed := fs.Bool("detailed", false, "also report on every tool use (PostToolUse)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}

	binPath, err := os.Executable()
	if err != nil {
		binPath = "claude-fleet"
	}
	configPath := os.Getenv("FLEET_CONFIG")

	switch sub {
	case "install":
		events := append([]string(nil), defaultEvents...)
		if *detailed {
			events = append(events, "PostToolUse")
		}
		path, err := install.Install(events, binPath, configPath, 5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hooks: %v\n", err)
			return 1
		}
		fmt.Printf("installed the %s reporting hooks in %s\n", legName(configPath), path)
		return 0
	case "uninstall":
		path, err := install.Uninstall(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hooks: %v\n", err)
			return 1
		}
		fmt.Printf("removed the %s reporting hooks from %s\n", legName(configPath), path)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown hooks subcommand: %s\n\n", sub)
		hooksUsage()
		return 2
	}
}

func legName(configPath string) string {
	if configPath == "" {
		return "production"
	}
	return "dev (FLEET_CONFIG=" + configPath + ")"
}

func hooksUsage() {
	fmt.Fprint(os.Stderr, `claude-fleet hooks — manage Claude Code reporting hooks (one leg per config)

Usage:
  claude-fleet hooks install [--detailed]   install the reporting hooks for this leg
  claude-fleet hooks uninstall              remove the reporting hooks for this leg

The leg is selected by FLEET_CONFIG: unset installs the production leg; set to a
config file installs a dev leg that reports to that server too. Legs are
independent — installing or removing one never touches the others.
`)
}
