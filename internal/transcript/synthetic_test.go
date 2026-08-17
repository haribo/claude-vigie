package transcript

import (
	"strings"
	"testing"
)

// #433: Claude Code writes `"model":"<synthetic>"` on an assistant line it
// generated itself instead of receiving from the API. Nothing filtered it, so the
// session's model became `<synthetic>` and stayed there until the next real
// assistant line — and every token produced meanwhile was attributed to that
// bucket by the daily rollups, which key on the session's model. A production day
// held `<synthetic> / output_tokens = 12879`: real output, stolen from the real
// model.
//
// The marker is not tied to API errors. Of the nine synthetic lines found across
// one machine's transcripts, only five carried `isApiErrorMessage: true`; the
// other four are ordinary ("No response requested."). So the discriminator has to
// be the model value itself, not the error flag.

func parseSyntheticLines(t *testing.T, lines ...string) *Info {
	t.Helper()
	p := NewParser()
	if err := p.Advance(strings.NewReader(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatalf("advance: %v", err)
	}
	return p.Info()
}

const realTurn = `{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50}}}`

// The API-error form, as observed.
const syntheticError = `{"type":"assistant","isApiErrorMessage":true,"apiErrorStatus":500,"message":{"id":"m2","model":"<synthetic>","stop_reason":"stop_sequence","usage":{"input_tokens":0,"output_tokens":0}}}`

// The non-error form — this is the one an isApiErrorMessage check would miss.
const syntheticQuiet = `{"type":"assistant","message":{"id":"m3","model":"<synthetic>","stop_reason":"stop_sequence","usage":{"input_tokens":0,"output_tokens":0}}}`

func TestSyntheticModelNeverBecomesTheSessionModel(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
	}{
		{"api error", syntheticError},
		{"no response requested", syntheticQuiet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := parseSyntheticLines(t, realTurn, tc.line)
			if info.Model != "claude-opus-4-8" {
				t.Errorf("model = %q, want the last real model kept", info.Model)
			}
		})
	}
}

// A synthetic line before any real one must leave the model unknown rather than
// naming a marker as the model.
func TestSyntheticModelAloneLeavesTheModelUnknown(t *testing.T) {
	if info := parseSyntheticLines(t, syntheticQuiet); info.Model != "" {
		t.Errorf("model = %q, want empty", info.Model)
	}
}

// The tokens produced *after* a synthetic line belong to the real model. This is
// the defect's actual cost: the rollups key on the session's model, so a stuck
// `<synthetic>` moved real output into a bucket that is not a model.
func TestOutputAfterASyntheticLineKeepsTheRealModel(t *testing.T) {
	info := parseSyntheticLines(t, realTurn, syntheticError,
		`{"type":"assistant","message":{"id":"m4","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":12829}}}`)

	if info.Model != "claude-opus-4-8" {
		t.Errorf("model = %q", info.Model)
	}
	if info.Usage.OutputTokens != 50+12829 {
		t.Errorf("output tokens = %d, want %d", info.Usage.OutputTokens, 50+12829)
	}
}

// Any bracketed marker is rejected, not just the one spelling observed: the point
// is that a marker is not a model.
func TestAnyBracketedMarkerIsRejected(t *testing.T) {
	info := parseSyntheticLines(t, realTurn,
		`{"type":"assistant","message":{"id":"m5","model":"<something-else>","stop_reason":"stop_sequence","usage":{"input_tokens":0,"output_tokens":0}}}`)
	if info.Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want the real model kept", info.Model)
	}
}

// Guard against over-rejecting: real model names must still be accepted, and a
// model change between turns must still be followed.
func TestRealModelsAreStillAccepted(t *testing.T) {
	info := parseSyntheticLines(t, realTurn,
		`{"type":"assistant","message":{"id":"m6","model":"claude-fable-5","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}}`)
	if info.Model != "claude-fable-5" {
		t.Errorf("model = %q, want the newer real model", info.Model)
	}
}

// The transient error status is driven by isApiErrorMessage and must keep working
// — rejecting the model must not disturb it (#279 / the `error` status).
func TestSyntheticErrorStillReportsTheAPIError(t *testing.T) {
	if info := parseSyntheticLines(t, realTurn, syntheticError); info.LastAPIError != 500 {
		t.Errorf("LastAPIError = %d, want 500", info.LastAPIError)
	}
	// A later real line clears it.
	if info := parseSyntheticLines(t, realTurn, syntheticError, realTurn); info.LastAPIError != 0 {
		t.Errorf("LastAPIError = %d after recovery, want 0", info.LastAPIError)
	}
}
