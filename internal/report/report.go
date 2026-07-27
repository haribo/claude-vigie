// Package report builds and sends a session event report to the fleet server.
// It is invoked by Claude Code hooks (client side) and must never block or
// fail the session: the caller ignores the returned error and always exits 0.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/config"
	"github.com/haribo/claude-fleet/internal/presence"
	"github.com/haribo/claude-fleet/internal/transcript"
)

// hookPayload is the JSON Claude Code passes to a command hook on stdin.
type hookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name"`
}

// Run reads a hook payload from stdin, builds a report for the given event
// (falling back to the payload's event name), and POSTs it to the server.
func Run(event string, stdin io.Reader) error {
	var p hookPayload
	if err := json.NewDecoder(stdin).Decode(&p); err != nil {
		return fmt.Errorf("decoding hook payload: %w", err)
	}
	if event == "" {
		event = p.HookEventName
	}
	if p.SessionID == "" || event == "" {
		return errors.New("hook payload missing session_id or event")
	}

	// Record/clear the session→process mapping so the watcher can tell a live
	// session from a closed one. Best-effort: a hook must never fail a session.
	recordPresence(event, p.SessionID)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	req := api.ReportRequest{
		Event:      event,
		SessionID:  p.SessionID,
		Machine:    cfg.Machine,
		ProjectDir: p.Cwd,
		GitBranch:  gitBranch(p.Cwd),
		LastTool:   p.ToolName,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	// The transcript is only worth reading at turn/session boundaries.
	if event == "Stop" || event == "SessionEnd" {
		if info, err := transcript.Parse(p.TranscriptPath); err == nil {
			req.Usage = &info.Usage
			req.Model = info.Model
			req.Title = info.Title
		}
	}

	return post(cfg, req)
}

// recordPresence captures the backing claude process at SessionStart and clears
// it at SessionEnd. Errors are ignored: presence is an enhancement, and the hook
// must exit 0 regardless (e.g. when not run under Claude Code, or off Linux).
func recordPresence(event, sessionID string) {
	switch event {
	case "SessionStart":
		if m, err := presence.ResolveClaude(); err == nil {
			_ = presence.Save(sessionID, m)
		}
	case "SessionEnd":
		_ = presence.Delete(sessionID)
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
		return fmt.Errorf("posting report: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

// gitBranch returns the current branch of the repo at dir, or "" if dir is not
// a git repo (best-effort context, never an error).
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
