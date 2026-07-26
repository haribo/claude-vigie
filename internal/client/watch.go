package client

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/haribo/claude-fleet/internal/config"
	"github.com/haribo/claude-fleet/internal/watch"
)

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	interval := fs.Duration("interval", 2*time.Second, "scan interval")
	maxAge := fs.Duration("max-age", 24*time.Hour, "ignore sessions inactive longer than this")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\nrun 'claude-fleet init' first\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "watching ~/.claude/projects (interval %s, max-age %s) — Ctrl+C to stop\n", *interval, *maxAge)
	if err := watch.Run(ctx, cfg, watch.Options{Interval: *interval, MaxAge: *maxAge}); err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		return 1
	}
	return 0
}
