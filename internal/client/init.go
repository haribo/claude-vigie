package client

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
)

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: vigie init\n\n"+
			"Connects this machine: asks for the server URL, the token and the machine\n"+
			"name, checks the connection, and writes the client config. Nothing else —\n"+
			"the watcher installs the reporting hooks and the call skill when it starts.\n")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	host, _ := os.Hostname()
	in, err := askSetup(host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 2
	}
	cfg := &config.Config{ServerURL: in.server, Token: in.token, Machine: in.machine}

	if err := testConnection(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "init: cannot reach server: %v\n", err)
		return 1
	}

	cfgPath, err := config.Save(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}

	fmt.Printf(`vigie configured:
  config:   %s
  server:   %s
  machine:  %s

Now start the watcher — or restart it if one is already running, since it reads
this config only at startup:

  systemctl --user restart vigie-watch    # or: vigie watch

It installs the reporting hooks and the call skill, and keeps them current.
`, cfgPath, cfg.ServerURL, cfg.Machine)
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
		return fmt.Errorf("%s responded 404 — not a vigie server (wrong port, or vigied not running there)", cfg.ServerURL)
	default:
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
}
