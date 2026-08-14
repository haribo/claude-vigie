// Package transcript parses a Claude Code session transcript (JSONL) to extract
// the session's identity, context, token usage, and current activity. Shared by
// the reporter (hook-driven) and the watcher; client-side only.
package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haribo/claude-vigie/internal/api"
)

// Info is what we extract from a transcript.
type Info struct {
	SessionID string
	Cwd       string
	GitBranch string
	Title     string
	Model     string
	// Effort is the reasoning effort of the last assistant line that carried one
	// (low/medium/high/xhigh/max). Claude Code writes it at the root of assistant
	// lines, but not on every line (older sessions and some message types omit it),
	// so the last non-empty value is kept rather than cleared — best-effort, same
	// caveat as the "thinking" status.
	Effort string
	// ContextTokens is the real prompt size of the latest main-thread request — the
	// last non-sidechain assistant line's input + cache-read + cache-creation
	// tokens. Compared against the model's window to show how full the context is
	// (#279). 0 when unknown.
	ContextTokens int64
	// PermissionMode is the session's last-seen permission mode — the canonical
	// `permissionMode` field (default/acceptEdits/plan/auto/bypassPermissions), kept
	// from the last line that carried it. The redundant type:"mode" line (whose
	// `normal` == `default`) is ignored. Empty when unknown (#303/#304).
	PermissionMode string
	Usage          api.Usage
	LastStopReason string // stop_reason of the last assistant message
	LastActivity   string // RFC3339 timestamp of the last line
	// LastAPIError is apiErrorStatus (HTTP code) of the last assistant line when
	// it was an API error (Claude Code writes isApiErrorMessage into the
	// transcript), else 0. A later non-error line clears it, so it is transient.
	LastAPIError int
	// Thinking is true when the last assistant line's final content block is a
	// thinking block — Claude is reasoning inside a turn, before it produces text
	// or a tool call. A later text/tool line clears it, so it is transient.
	Thinking bool
	// Activity is a short message for the last tool the session ran (its most
	// recent tool_use block), else "" — the watcher's fallback for the "doing"
	// column when the PostToolUse hook did not report it.
	Activity string
	// PendingTool is the name of the most recent foreground tool_use with no
	// matching tool_result — a tool genuinely still awaiting a result. Empty when
	// every tool_use has been answered. Used to detect a stalled turn (#256).
	PendingTool string
	// BackgroundActive is true when an unresolved tool_use is a backgrounded Bash
	// (run_in_background) — a real background task still running, which legitimately
	// keeps the session working rather than stalling it (#256).
	BackgroundActive bool
	// AgentsActive is the number of async subagents (Task/Agent) launched from this
	// transcript and not yet reported finished by a <task-notification>. A session
	// whose only work runs in a subagent would otherwise read idle (#344).
	AgentsActive int
	// AgentActivity is the "doing" line for those agents, e.g. "2 agents: <desc>".
	AgentActivity string
	// LastCompactBoundary is the RFC3339 timestamp of the last `compact_boundary`
	// system line — the moment a context compaction finished. The watcher uses it
	// to close a `compacting` status (#342). Empty when the session never compacted.
	LastCompactBoundary string
	// Interrupted is true when the last non-system message is Claude Code's
	// synthetic "[Request interrupted by user]" line — the operator killed the
	// turn (Ctrl-C/Esc). The base status is still idle; the watcher only sets the
	// DETAIL marker to "interrupted", cleared by the next real message (#351).
	Interrupted bool
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type message struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	StopReason string          `json:"stop_reason"`
	Usage      usage           `json:"usage"`
	Content    json.RawMessage `json:"content"` // array of blocks (assistant); string (user) — parsed lazily
}

type line struct {
	Type           string  `json:"type"`
	Subtype        string  `json:"subtype"` // system lines: "compact_boundary" ends a compaction (#342)
	SessionID      string  `json:"sessionId"`
	Cwd            string  `json:"cwd"`
	GitBranch      string  `json:"gitBranch"`
	CustomTitle    string  `json:"customTitle"`
	AiTitle        string  `json:"aiTitle"`
	Timestamp      string  `json:"timestamp"`
	Effort         string  `json:"effort"`         // reasoning effort, root-level on assistant lines
	PermissionMode string  `json:"permissionMode"` // permission mode, on user / permission-mode lines (#304)
	IsAPIError     bool    `json:"isApiErrorMessage"`
	IsSidechain    bool    `json:"isSidechain"` // a sub-agent line — excluded from context sizing (#279)
	APIErrorStatus int     `json:"apiErrorStatus"`
	Message        message `json:"message"`
}

// Parse reads the whole transcript at path and returns the extracted Info. Token
// usage is summed over assistant messages (deduplicated by message id, since
// retries repeat a line); the title is customTitle (/rename) if present, else
// aiTitle; LastStopReason is the stop_reason of the last assistant message. It is
// a convenience wrapper over Parser for one-shot callers (the reporter); the
// watcher parses incrementally (see Parser, #257).
func Parse(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer func() { _ = f.Close() }()
	p := NewParser()
	if err := p.Advance(f); err != nil {
		return nil, err
	}
	return p.Info(), nil
}

func (info *Info) applyMeta(l line, titles *titleTracker) {
	if l.SessionID != "" {
		info.SessionID = l.SessionID
	}
	if l.Cwd != "" {
		info.Cwd = l.Cwd
	}
	if l.GitBranch != "" {
		info.GitBranch = l.GitBranch
	}
	if l.Timestamp != "" {
		info.LastActivity = l.Timestamp
	}
	if l.PermissionMode != "" {
		info.PermissionMode = l.PermissionMode // keep the last non-empty (#304)
	}
	titles.observe(l.CustomTitle, l.AiTitle)
}

func (info *Info) applyAssistant(l line, seen map[string]bool) {
	m := l.Message
	info.LastStopReason = m.StopReason
	// Track the last assistant line's API-error state (set before the retry
	// dedup below, which only guards token accumulation): a later non-error line
	// clears it, so a recovered session stops reporting the error.
	if l.IsAPIError {
		info.LastAPIError = l.APIErrorStatus
	} else {
		info.LastAPIError = 0
	}
	info.Thinking = lastBlockIsThinking(m.Content)
	info.Activity = lastToolActivity(m.Content)
	if isRealModel(m.Model) {
		info.Model = m.Model // a marker never becomes the session's model (#433)
	}
	if l.Effort != "" {
		info.Effort = l.Effort // keep the last reported effort; a line without it does not clear it
	}
	// Context size: the real prompt of the latest main-thread request. Skip
	// sub-agent (sidechain) lines and lines with no usage (e.g. API errors), so the
	// value tracks the main conversation and never dips to a partial number (#279).
	if !l.IsSidechain {
		if ctx := m.Usage.InputTokens + m.Usage.CacheReadInputTokens + m.Usage.CacheCreationInputTokens; ctx > 0 {
			info.ContextTokens = ctx
		}
	}
	if m.ID != "" {
		if seen[m.ID] {
			return // retry of an already-counted message
		}
		seen[m.ID] = true
	}
	info.Usage.InputTokens += m.Usage.InputTokens
	info.Usage.OutputTokens += m.Usage.OutputTokens
	info.Usage.CacheCreationTokens += m.Usage.CacheCreationInputTokens
	info.Usage.CacheReadTokens += m.Usage.CacheReadInputTokens
}

// isRealModel reports whether an assistant line's `model` field names an actual
// model. Claude Code writes bracketed markers there for lines it generated itself
// rather than received from the API — `<synthetic>` is the one observed, on both
// API-error lines and ordinary ones ("No response requested."). Letting a marker
// through made it the session's model until the next real turn, and the daily
// rollups key on that, so real output was attributed to a bucket that is not a
// model (#433).
//
// The check is the bracket, not the error flag: of nine synthetic lines found
// across one machine's transcripts, only five were flagged as API errors.
func isRealModel(m string) bool {
	return m != "" && !strings.HasPrefix(m, "<")
}

// lastBlockIsThinking reports whether an assistant message's content ends with a
// thinking block. Content is a raw array of {type,...} blocks; it is parsed
// lazily here so a non-array content (e.g. a user string) never fails the line.
func lastBlockIsThinking(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var blocks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil || len(blocks) == 0 {
		return false
	}
	return blocks[len(blocks)-1].Type == "thinking"
}

// interruptMarkers are Claude Code's synthetic user lines injected when the
// operator interrupts a turn (Ctrl-C/Esc). Undocumented internals (#351).
var interruptMarkers = map[string]bool{
	"[Request interrupted by user]":              true,
	"[Request interrupted by user for tool use]": true,
}

// isInterruptLine reports whether a user message's content is a synthetic
// interrupt marker. The marker's content is a block array with a text block; a
// real typed prompt is a plain JSON string, which fails this decode — so a user
// typing the literal marker text does not false-positive (#351).
func isInterruptLine(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return false // not an array (e.g. a plain-string prompt)
	}
	for _, b := range blocks {
		if b.Type == "text" && interruptMarkers[b.Text] {
			return true
		}
	}
	return false
}

// lastToolActivity returns a short message for the most recent tool_use block in
// an assistant message's content, else "" (e.g. a text-only reply).
func lastToolActivity(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == "tool_use" {
			return ToolActivity(blocks[i].Name, blocks[i].Input)
		}
	}
	return ""
}

// activityMax bounds an activity message at the source; the TUI column truncates
// further to its width.
const activityMax = 80

// ToolActivity renders a short, human "doing" message for a tool call from its
// name and input JSON, capped at activityMax. Shared by the reporter (hook
// tool_input) and the watcher (transcript tool_use block).
func ToolActivity(tool string, input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
		FilePath    string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)

	var s string
	switch tool {
	case "Bash":
		if in.Description != "" {
			s = "Bash: " + in.Description
		} else {
			s = "Bash"
		}
	case "Edit", "Write", "Read", "NotebookEdit":
		if in.FilePath != "" {
			s = tool + " " + filepath.Base(in.FilePath)
		} else {
			s = tool
		}
	case "Task", "Agent":
		if in.Description != "" {
			s = in.Description
		} else {
			s = "Agent"
		}
	default:
		s = tool
	}
	return capActivity(s)
}

func capActivity(s string) string {
	r := []rune(s)
	if len(r) > activityMax {
		return string(r[:activityMax-1]) + "…"
	}
	return s
}

// titleTracker keeps the latest custom (/rename) and auto titles seen.
type titleTracker struct {
	custom string
	ai     string
}

func (t *titleTracker) observe(custom, ai string) {
	if custom != "" {
		t.custom = custom
	}
	if ai != "" {
		t.ai = ai
	}
}

func (t *titleTracker) resolve() string {
	if t.custom != "" {
		return t.custom
	}
	return t.ai
}
