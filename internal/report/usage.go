package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/haribo/claude-fleet/internal/api"
)

// transcriptLine is the subset of a transcript JSONL line we care about.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// maxTranscriptLine bounds a single JSONL line; transcript messages can be
// large (embedded tool output), so allow well beyond bufio's 64K default.
const maxTranscriptLine = 16 * 1024 * 1024

// aggregateUsage sums token usage across assistant messages in the transcript,
// deduplicating by message id (retries repeat a line), and returns the last
// model seen. Each assistant line is a distinct API call, so summing reflects
// the tokens actually consumed over the session.
func aggregateUsage(path string) (*api.Usage, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("opening transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	var u api.Usage
	var model string
	seen := make(map[string]bool)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxTranscriptLine)
	for sc.Scan() {
		var line transcriptLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue // skip malformed lines rather than fail the whole report
		}
		if line.Type != "assistant" {
			continue
		}
		m := line.Message
		if m.ID != "" {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
		}
		u.InputTokens += m.Usage.InputTokens
		u.OutputTokens += m.Usage.OutputTokens
		u.CacheCreationTokens += m.Usage.CacheCreationInputTokens
		u.CacheReadTokens += m.Usage.CacheReadInputTokens
		if m.Model != "" {
			model = m.Model
		}
	}
	if err := sc.Err(); err != nil {
		return nil, "", fmt.Errorf("reading transcript: %w", err)
	}
	return &u, model, nil
}
