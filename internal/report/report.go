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
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/compaction"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/localwatch"
	"github.com/haribo/claude-vigie/internal/presence"
	"github.com/haribo/claude-vigie/internal/reachability"
	"github.com/haribo/claude-vigie/internal/transcript"
)

// Seams for the transcript decision: tests drive both sides of it without a real
// watcher and without a large file on disk.
var (
	parseTranscript = transcript.Parse
	watcherLive     = localwatch.Live
)

// hookPayload is the JSON Claude Code passes to a command hook on stdin.
type hookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name"`
	// PermissionMode is the instant source for the session's permission mode; it
	// rides UserPromptSubmit / PreToolUse payloads (absent on Notification) (#304).
	PermissionMode string `json:"permission_mode"`
	// NotificationType is set on Notification events (permission_prompt,
	// idle_prompt, …); it lets the server tell "waiting on a human" from "idle".
	NotificationType string `json:"notification_type"`
	// ToolInput is the tool's input JSON on PostToolUse; Message is the
	// human-readable text on Notification — both feed the "doing" activity.
	ToolInput json.RawMessage `json:"tool_input"`
	Message   string          `json:"message"`
	// Trigger is "auto" | "manual" on a PreCompact event (#342).
	Trigger string `json:"trigger"`
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

	// Only Claude Code reports (ADR-0013). Refused before anything else runs: a
	// foreign harness must leave no presence mapping, no compaction marker and no
	// row — see requireClaudeCode for why the check cannot live on the server.
	if err := requireClaudeCode(p.SessionID); err != nil {
		recordRefusal()
		return err
	}

	// Record/clear the session→process mapping so the watcher can tell a live
	// session from a closed one. Best-effort: a hook must never fail a session.
	recordPresence(event, p.SessionID)

	// PreCompact drops a marker the watcher reads to refine `working` →
	// `compacting`; no server report is needed and none is sent (the watcher
	// reports the status). Best-effort — a hook must never fail a session
	// (#342, ADR-0008).
	if event == "PreCompact" {
		_ = compaction.Save(p.SessionID, compaction.Marker{
			Started: clock.Now().UTC().Format(time.RFC3339),
			Trigger: p.Trigger,
		})
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	req := api.ReportRequest{
		Event:            event,
		SessionID:        p.SessionID,
		User:             systemUser(),
		Machine:          cfg.Machine,
		ProjectDir:       p.Cwd,
		GitBranch:        gitBranch(p.Cwd),
		LastTool:         p.ToolName,
		NotificationType: p.NotificationType,
		PermissionMode:   p.PermissionMode, // "" when the event doesn't carry it
		Detail:           hookActivity(event, p),
		Timestamp:        clock.Now().UTC().Format(time.RFC3339),
	}
	// The transcript is only worth reading at turn/session boundaries — and only
	// when nothing else has already read it. A local watcher parses the same file
	// incrementally every ~2 s and reports a superset of these fields, so on a
	// watched machine this read is a full O(file) re-parse of bytes the server
	// already has, inside a hook the session waits on. Every field below is
	// "absent → keep the last known" server-side, so omitting them erases nothing
	// (#420, docs/design/transcript-reads.md).
	if (event == "Stop" || event == "SessionEnd") && !watcherLive(clock.Now()) {
		if info, err := parseTranscript(p.TranscriptPath); err == nil {
			req.Usage = &info.Usage
			req.Model = info.Model
			req.Effort = info.Effort
			ctx := info.ContextTokens // a parsed reading — known, even at 0 (#367)
			req.ContextTokens = &ctx
			if info.PermissionMode != "" {
				req.PermissionMode = info.PermissionMode
			}
			req.Title = info.Title
		}
	}

	return post(cfg, req)
}

// sessionIDEnv is the session id Claude Code exports into every tool process it
// spawns. A subagent inherits the *same* value as its parent — verified against a
// live subagent — so a call raised from one resolves to the parent session with no
// extra work. (`CLAUDE_CODE_CHILD_SESSION` is a boolean flag, not a child id, and
// carries nothing usable here.)
const sessionIDEnv = "CLAUDE_CODE_SESSION_ID"

// Call raises a call for the operator on the session this process runs in
// (ADR-0010). The message is optional: a call with no message is still a call.
//
// Like the hooks, it is best-effort — the caller ignores the error and exits 0, so
// a monitoring signal can never fail the operator's session.
func Call(message string) error {
	sessionID := os.Getenv(sessionIDEnv) //nolint:forbidigo // the session id is Claude Code's own handle, not vigie config
	if sessionID == "" {
		return fmt.Errorf("%s is not set — `vigie call` runs inside a Claude Code session", sessionIDEnv)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return post(cfg, api.ReportRequest{
		Event:       "call",
		SessionID:   sessionID,
		User:        systemUser(),
		Machine:     cfg.Machine,
		CallMessage: message,
		Timestamp:   clock.Now().UTC().Format(time.RFC3339),
	})
}

// hookActivity extracts the short "doing" message: the tool call on
// PostToolUse, the notification text on Notification, else "".
func hookActivity(event string, p hookPayload) string {
	switch event {
	case "PostToolUse":
		return transcript.ToolActivity(p.ToolName, p.ToolInput)
	case "Notification":
		// An idle_prompt notification lands the session on idle (#206); a waiting
		// message there would contradict the status, so carry no activity.
		if p.NotificationType == "idle_prompt" {
			return ""
		}
		r := []rune(p.Message)
		if len(r) > 80 {
			return string(r[:79]) + "…"
		}
		return p.Message
	}
	return ""
}

// recordPresence captures the backing claude process at SessionStart and clears
// it at SessionEnd. It also refreshes the mapping on UserPromptSubmit, so a
// session already open when the hook was installed gets backfilled on its next
// message (SessionStart does not replay for a running session). Errors are
// ignored: presence is an enhancement, and the hook must exit 0 regardless
// (e.g. when not run under Claude Code, or off Linux).
func recordPresence(event, sessionID string) {
	switch event {
	case "SessionStart", "UserPromptSubmit":
		if m, err := presence.ResolveClaude(); err == nil {
			_ = presence.Save(sessionID, m)
		}
	case "SessionEnd":
		_ = presence.Delete(sessionID)
	}
}

// httpClient carries a timeout (http.DefaultClient has none); the request also
// sets a context deadline.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// postTimeout bounds the one request a hook makes. It is a var, not a const, so
// a test can shrink it: the deadline is the cost this package exists to bound,
// and a test asserting on it should not have to wait it out.
var postTimeout = 3 * time.Second

func post(cfg *config.Config, req api.ReportRequest) error {
	// A hook must not wait on a daemon that has already been found unreachable:
	// the deadline below is paid per event, and a black-holing daemon charges it
	// on every tool call (docs/design/unreachable-daemon.md, #578).
	if reachability.Unreachable(cfg.ServerURL, clock.Now()) {
		return fmt.Errorf("not posting: %s was unreachable less than %s ago", cfg.ServerURL, reachability.StaleAfter)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/report"

	ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		// Best-effort, like every other side effect here: failing to record the
		// failure costs the next hook one deadline, and must not add an error of
		// its own to a path that already has one.
		_ = reachability.Mark(cfg.ServerURL, clock.Now(), err)
		return fmt.Errorf("posting report: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// It answered, so it is reachable — whatever it answered. A refusal is about
	// the report's content (drift, validation), not about reaching the daemon,
	// and must not keep the next report from being sent.
	_ = reachability.Clear(cfg.ServerURL)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

// systemUser returns the OS account running the session: the USER env var if
// set, else the current user, else "" (best-effort context, never an error).
func systemUser() string {
	if u := config.OSUser(); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// gitBranchTimeout bounds the branch lookup. A healthy `rev-parse` answers in
// milliseconds; a second is far past anything that is still working, and far
// short of the 5 s the hook has in total.
const gitBranchTimeout = time.Second

// gitBranchWaitDelay bounds the wait on the output pipes once the deadline has
// fired and the process has been killed. See the comment in gitBranch.
const gitBranchWaitDelay = 200 * time.Millisecond

// gitBranch returns the current branch of the repo at dir, or "" if dir is not
// a git repo (best-effort context, never an error).
//
// The call is bounded because of where it runs: every hook stamps this field,
// `PostToolUse` is installed by default, and the hook budget vigie sets for
// itself is 5 s. An `index.lock` held by another process, or a repository on a
// stalled mount, would otherwise spend that budget on decoration — Claude Code
// kills the hook and the report goes with it, transition and heartbeat included
// (#658, the class docs/design/transcript-reads.md keeps off this path).
//
// A timeout is answered like any other failure: no branch. It already is
// best-effort context, and a report without it erases nothing server-side.
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitBranchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	// The deadline alone does not bound the call. Canceling kills `git`, but
	// `Output` waits on the stdout pipe, and a grandchild `git` spawned (a
	// credential helper, a pager, a hook of its own) inherits that pipe and holds
	// it open after its parent dies. WaitDelay is what closes it, and without it
	// the timeout is advisory — the regression test hangs the full budget with the
	// context in place and no WaitDelay.
	cmd.WaitDelay = gitBranchWaitDelay
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
