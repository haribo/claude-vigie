package client

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/config"
	"github.com/haribo/claude-fleet/internal/install"
)

// defaultEvents are the hooks installed by default. PostToolUse is included so
// the dashboard can show a live "doing" message and an activity heartbeat; it
// fires per tool use, which is the intended trade-off.
var defaultEvents = []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Notification", "Stop", "SessionEnd"}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	server := fs.String("server", "", "fleet server URL to report to")
	token := fs.String("token", "", "shared auth token")
	machine := fs.String("machine", "", "machine name (defaults to the hostname)")
	uninstall := fs.Bool("uninstall", false, "remove claude-fleet hooks and stop reporting")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *uninstall {
		path, err := install.Uninstall("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "init: %v\n", err)
			return 1
		}
		fmt.Printf("removed claude-fleet hooks from %s\n", path)
		return 0
	}

	if *server == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "init: --server and --token are required")
		return 2
	}

	mach := *machine
	if mach == "" {
		if h, err := os.Hostname(); err == nil {
			mach = h
		}
	}
	cfg := &config.Config{ServerURL: *server, Token: *token, Machine: mach}

	if err := testConnection(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "init: cannot reach server: %v\n", err)
		return 1
	}

	cfgPath, err := config.Save(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}

	binPath, err := os.Executable()
	if err != nil {
		binPath = "claude-fleet"
	}

	settingsPath, err := install.Install(defaultEvents, binPath, "", 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}

	fmt.Printf(`claude-fleet configured:
  config:   %s
  hooks:    %s
  server:   %s
  machine:  %s

New Claude Code sessions on this machine will report to the fleet.
`, cfgPath, settingsPath, cfg.ServerURL, mach)
	return 0
}

// testConnection verifies the server is reachable and the token is accepted.
// httpClient carries a timeout (http.DefaultClient has none); the request also
// sets a context deadline.
var httpClient = &http.Client{Timeout: 10 * time.Second}

func testConnection(cfg *config.Config) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/sessions"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("invalid token")
	case http.StatusNotFound:
		return fmt.Errorf("%s responded 404 — not a claude-fleet server (wrong port, or claude-fleetd not running there)", cfg.ServerURL)
	default:
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
}
