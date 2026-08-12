package client

import (
	"flag"
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/install"
)

// runHooks installs or removes one reporting leg. The leg is selected by
// VIGIE_CONFIG (or the deprecated FLEET_CONFIG): unset is the production leg; a
// path is a dev leg that reports to that config's server alongside production.
// Legs are independent.
func runHooks(args []string) int {
	if len(args) == 0 {
		hooksUsage()
		return 2
	}
	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("hooks", flag.ContinueOnError)
	fs.Usage = hooksUsage // an unknown flag prints the real usage, not a bare "Usage of hooks:" (#354)
	if err := fs.Parse(rest); err != nil {
		return 2
	}

	binPath, err := os.Executable()
	if err != nil {
		binPath = "vigie"
	}
	configPath := config.EnvConfigPath()

	switch sub {
	case "install":
		path, err := install.Install(defaultEvents, binPath, configPath, 5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hooks: %v\n", err)
			return 1
		}
		fmt.Printf("installed the %s reporting hooks in %s\n", legName(configPath), path)
		// The skill teaches Claude that `vigie call` exists (#391). It is not
		// leg-scoped — there is one Claude Code configuration per machine — so only
		// the production leg writes it, leaving the dev leg free of side effects.
		if configPath == "" {
			if sp, sErr := install.InstallSkill(); sErr != nil {
				fmt.Fprintf(os.Stderr, "hooks: installing the call skill failed (continuing): %v\n", sErr)
			} else {
				fmt.Printf("installed the call skill in %s\n", sp)
			}
		}
		return 0
	case "uninstall":
		path, err := install.Uninstall(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hooks: %v\n", err)
			return 1
		}
		fmt.Printf("removed the %s reporting hooks from %s\n", legName(configPath), path)
		if configPath == "" { // symmetric with install: the dev leg owns no skill
			if sp, sErr := install.UninstallSkill(); sErr != nil {
				fmt.Fprintf(os.Stderr, "hooks: removing the call skill failed: %v\n", sErr)
			} else {
				fmt.Printf("removed the call skill from %s\n", sp)
			}
		}
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
	return "dev (VIGIE_CONFIG=" + configPath + ")"
}

func hooksUsage() {
	fmt.Fprint(os.Stderr, `vigie hooks — manage Claude Code reporting hooks (one leg per config)

Usage:
  vigie hooks install     install the reporting hooks for this leg
  vigie hooks uninstall   remove the reporting hooks for this leg

The leg is selected by VIGIE_CONFIG (or the deprecated FLEET_CONFIG): unset installs
the production leg; set to a config file installs a dev leg that reports to that
server too. Legs are independent — installing or removing one never touches the others.
`)
}
