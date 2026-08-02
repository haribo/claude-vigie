package transcript

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

// TestIncrementalEqualsFull replays the same transcript two ways — one Advance
// over the whole thing, versus two Advances split mid-line with a seek to Offset
// in between (exactly what the watcher does) — and asserts the resulting Info is
// identical, so the incremental parse (#257) is lossless.
func TestIncrementalEqualsFull(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make"}}]}}`,
		`{"type":"user","cwd":"/work","gitBranch":"main","timestamp":"2026-08-02T10:00:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1"}]}}`,
		`{"type":"assistant","message":{"id":"m2","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":200,"output_tokens":80}}}`,
		`{"type":"assistant","message":{"id":"m2","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":200,"output_tokens":80}}}`, // retry: must dedup
		`{"type":"assistant","message":{"id":"m3","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/x"}}]}}`,
		`{"customTitle":"my-conv"}`,
	}
	data := []byte(strings.Join(lines, "\n") + "\n")

	full := NewParser()
	if err := full.Advance(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	inc := NewParser()
	r := bytes.NewReader(data)
	if err := inc.Advance(io.LimitReader(r, int64(len(data)/2))); err != nil { // stop mid-line
		t.Fatal(err)
	}
	if _, err := r.Seek(inc.Offset(), io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := inc.Advance(r); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(full.Info(), inc.Info()) {
		t.Errorf("incremental != full:\n full=%+v\n  inc=%+v", full.Info(), inc.Info())
	}
	// Sanity: the fixture exercises dedup (m2 once), the pending tool, and title.
	got := full.Info()
	if got.Usage.OutputTokens != 130 || got.Title != "my-conv" || got.PendingTool != "Read" {
		t.Errorf("fixture wrong: out=%d title=%q pending=%q", got.Usage.OutputTokens, got.Title, got.PendingTool)
	}
}
