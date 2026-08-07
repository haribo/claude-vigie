package transcript

import (
	"path/filepath"
	"testing"
)

// TestInterruptMarker is the #351 signal: the synthetic "[Request interrupted by
// user]" user line (a block array) sets Interrupted; a following system line does
// not clear it, a real message does, and a plain-string prompt never matches.
func TestInterruptMarker(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jsonl")

	// Interrupted mid-tool: an is_error tool_result closes the pending tool, the
	// synthetic marker follows, then a turn_duration system line — must stay set.
	writeJSONL(t, p,
		`{"type":"assistant","message":{"id":"m1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"interrupted"}]}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"system","subtype":"turn_duration"}`,
	)
	if info, err := Parse(p); err != nil {
		t.Fatal(err)
	} else if !info.Interrupted {
		t.Error("expected Interrupted after the marker (a system line must not clear it)")
	}

	// A real prompt after the marker clears it.
	writeJSONL(t, p,
		`{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"type":"user","message":{"role":"user","content":"carry on"}}`,
	)
	if info, err := Parse(p); err != nil {
		t.Fatal(err)
	} else if info.Interrupted {
		t.Error("a real user prompt must clear the interrupt marker")
	}

	// A typed prompt equal to the marker text is a plain string, not an array →
	// never a false positive.
	writeJSONL(t, p, `{"type":"user","message":{"content":"[Request interrupted by user]"}}`)
	if info, err := Parse(p); err != nil {
		t.Fatal(err)
	} else if info.Interrupted {
		t.Error("a plain-string prompt must not false-positive as an interrupt")
	}
}
