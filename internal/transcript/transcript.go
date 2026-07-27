// Package transcript parses a Claude Code session transcript (JSONL) to extract
// the session's identity, context, token usage, and current activity. Shared by
// the reporter (hook-driven) and the watcher; client-side only.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/haribo/claude-fleet/internal/api"
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
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type message struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      usage  `json:"usage"`
}

type line struct {
	Type        string  `json:"type"`
	SessionID   string  `json:"sessionId"`
	Cwd         string  `json:"cwd"`
	GitBranch   string  `json:"gitBranch"`
	CustomTitle string  `json:"customTitle"`
	AiTitle     string  `json:"aiTitle"`
	Timestamp   string  `json:"timestamp"`
	Message     message `json:"message"`
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

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue // skip malformed lines rather than fail the whole parse
		}
		info.applyMeta(l, &titles)
		if l.Type == "assistant" {
			info.applyAssistant(l.Message, seen)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading transcript: %w", err)
	}

	info.Title = titles.resolve()
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

func (info *Info) applyAssistant(m message, seen map[string]bool) {
	info.LastStopReason = m.StopReason
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
