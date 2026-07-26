// Package watch continuously scans the local Claude Code transcripts and reports
// every recent session to the fleet server, deriving status from the transcript
// state. Client-side; it covers sessions the hooks miss (already-open ones).
package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/config"
	"github.com/haribo/claude-fleet/internal/transcript"
)

// Options configures the watch loop.
type Options struct {
	Interval time.Duration
	MaxAge   time.Duration
}

// Status thresholds derived from how recently a transcript changed.
const (
	activeWindow  = 10 * time.Second
	waitingWindow = 15 * time.Minute
)

// ProjectsDir returns the Claude Code transcripts root (~/.claude/projects).
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Run scans on an interval and reports until ctx is canceled.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	root, err := ProjectsDir()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		reports, err := Scan(root, cfg.Machine, opts.MaxAge, time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		}
		for _, r := range reports {
			if err := post(cfg, r); err != nil {
				fmt.Fprintf(os.Stderr, "watch: reporting %s: %v\n", r.SessionID, err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Scan reads every transcript under root modified within maxAge and returns a
// report (with a derived status) for each.
func Scan(root, machine string, maxAge time.Duration, now time.Time) ([]api.ReportRequest, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("globbing transcripts: %w", err)
	}

	var reports []api.ReportRequest
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		age := now.Sub(fi.ModTime())
		if age > maxAge {
			continue
		}
		info, err := transcript.Parse(p)
		if err != nil {
			continue
		}

		id := info.SessionID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(p), ".jsonl")
		}
		usage := info.Usage
		reports = append(reports, api.ReportRequest{
			Event:      "watch",
			SessionID:  id,
			Machine:    machine,
			ProjectDir: info.Cwd,
			GitBranch:  info.GitBranch,
			Model:      info.Model,
			Title:      info.Title,
			Status:     deriveStatus(info.LastStopReason, age),
			Usage:      &usage,
			Timestamp:  fi.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return reports, nil
}

// deriveStatus maps a transcript's last stop_reason and age to a status.
func deriveStatus(lastStopReason string, age time.Duration) string {
	switch {
	case lastStopReason == "tool_use" || age < activeWindow:
		return "working"
	case age < waitingWindow:
		return "waiting"
	default:
		return "idle"
	}
}

func post(cfg *config.Config, req api.ReportRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/report"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}
