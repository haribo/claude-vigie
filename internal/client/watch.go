package client

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/install"
	"github.com/haribo/claude-vigie/internal/watch"
)

// refreshHooks re-installs the watcher's own reporting leg so the installed hooks
// always match this binary and its default event set (ADR-0009). Best-effort: a
// failure (unreadable/malformed settings.json) is logged and the watch proceeds.
func refreshHooks() {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "vigie"
	}
	path, err := install.Install(defaultEvents, binPath, config.EnvConfigPath(), 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: refreshing hooks failed (continuing): %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "watch: hooks refreshed in %s\n", path)
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	interval := fs.Duration("interval", 2*time.Second, "scan interval")
	maxAge := fs.Duration("max-age", 24*time.Hour, "ignore sessions inactive longer than this")
	usageInterval := fs.Duration("usage-interval", 5*time.Minute, "subscription usage fetch interval (0 to disable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\nrun 'vigie init' first\n", err)
		return 1
	}

	// The watcher owns the hooks lifecycle: refresh its own leg at startup so the
	// installed hooks always match this binary and event set (ADR-0009, #355).
	// Best-effort — a hooks problem must never stop the watch.
	refreshHooks()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "watching ~/.claude/projects (interval %s, max-age %s) — Ctrl+C to stop\n", *interval, *maxAge)
	opts := watch.Options{Interval: *interval, MaxAge: *maxAge, UsageInterval: *usageInterval}
	if err := watch.Run(ctx, cfg, opts); err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		return 1
	}
	return 0
}
