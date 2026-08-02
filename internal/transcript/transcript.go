// Package transcript parses a Claude Code session transcript (JSONL) to extract
// the session's identity, context, token usage, and current activity. Shared by
// the reporter (hook-driven) and the watcher; client-side only.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/haribo/claude-vigie/internal/api"
)

// Info is what we extract from a transcript.
type Info struct {
	SessionID      string
	Cwd            string
	GitBranch      string
	Title          string
	Model          string
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
	SessionID      string  `json:"sessionId"`
	Cwd            string  `json:"cwd"`
	GitBranch      string  `json:"gitBranch"`
	CustomTitle    string  `json:"customTitle"`
	AiTitle        string  `json:"aiTitle"`
	Timestamp      string  `json:"timestamp"`
	IsAPIError     bool    `json:"isApiErrorMessage"`
	APIErrorStatus int     `json:"apiErrorStatus"`
	Message        message `json:"message"`
}

// maxLine bounds a single JSONL line; transcript messages can be large
// (embedded tool output), so allow well beyond bufio's 64K default.
const maxLine = 16 * 1024 * 1024

// Parse reads the transcript at path and returns the extracted Info. Token
// usage is summed over assistant messages (deduplicated by message id, since
// retries repeat a line). The title is customTitle (/rename) if present, else
// aiTitle. LastStopReason is the stop_reason of the last assistant message.
func Parse(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	var info Info
	var titles titleTracker
	seen := make(map[string]bool)
	pending := newPendingTools()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue // skip malformed lines rather than fail the whole parse
		}
		info.applyMeta(l, &titles)
		switch l.Type {
		case "assistant":
			info.applyAssistant(l, seen)
			pending.addToolUses(l.Message.Content)
		case "user":
			pending.clearToolResults(l.Message.Content)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading transcript: %w", err)
	}

	info.Title = titles.resolve()
	info.PendingTool, info.BackgroundActive = pending.resolve()
	return &info, nil
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
	if m.Model != "" {
		info.Model = m.Model
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
