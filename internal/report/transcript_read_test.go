package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/transcript"
)

// countingParse replaces the transcript read and records how often it happened.
// The bug in #420 is not a wrong value — it is doing this work at all.
func countingParse(t *testing.T) *int {
	t.Helper()
	n := 0
	orig := parseTranscript
	parseTranscript = func(string) (*transcript.Info, error) {
		n++
		return &transcript.Info{Model: "m"}, nil
	}
	t.Cleanup(func() { parseTranscript = orig })
	return &n
}

func withWatcher(t *testing.T, live bool) {
	t.Helper()
	orig := watcherLive
	watcherLive = func(time.Time) bool { return live }
	t.Cleanup(func() { watcherLive = orig })
}

func runStop(t *testing.T) api.ReportRequest {
	t.Helper()
	asClaudeCode(t, "s1")
	var got api.ReportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("HOME", t.TempDir())
	writeConfig(t, srv.URL)

	payload := `{"session_id":"s1","cwd":"/p","hook_event_name":"Stop","transcript_path":"/does/not/matter.jsonl"}`
	if err := Run("Stop", strings.NewReader(payload)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return got
}

// TestStopSkipsTheTranscriptWhenAWatcherIsLive is #420. The hook used to re-read
// the whole transcript at the end of every turn; measured against the largest
// real transcript on a development machine (584.7 MB) that took 11.1 s, against a
// 5 s hook timeout — so the report was killed *and* the operator's session stalled
// after every turn. A live local watcher has already parsed the same file
// incrementally, so the read is pure waste.
func TestStopSkipsTheTranscriptWhenAWatcherIsLive(t *testing.T) {
	n := countingParse(t)
	withWatcher(t, true)

	got := runStop(t)

	if *n != 0 {
		t.Errorf("the transcript was read %d time(s) although a watcher is live", *n)
	}
	if got.Event != "Stop" {
		t.Errorf("event = %q — the report itself must still be sent", got.Event)
	}
}

// TestStopReadsTheTranscriptWithoutAWatcher is the other half, and the reason the
// read cannot simply be deleted: a hooks-only machine (ADR-0009 keeps it
// supported) has no other source for these fields.
func TestStopReadsTheTranscriptWithoutAWatcher(t *testing.T) {
	n := countingParse(t)
	withWatcher(t, false)

	got := runStop(t)

	if *n != 1 {
		t.Errorf("the transcript was read %d time(s), want exactly 1 with no watcher", *n)
	}
	if got.Model != "m" {
		t.Errorf("model = %q, want the parsed value to still reach the server", got.Model)
	}
}

// TestOmittedFieldsAreAbsentNotEmpty: skipping the read must leave the fields out
// of the payload entirely. The server keeps the last known value for an absent
// field, so an explicit zero would erase what the watcher reported.
func TestOmittedFieldsAreAbsentNotEmpty(t *testing.T) {
	countingParse(t)
	withWatcher(t, true)

	got := runStop(t)

	if got.Usage != nil {
		t.Errorf("usage = %+v, want nil so the server keeps the watcher's figure", *got.Usage)
	}
	if got.ContextTokens != nil {
		t.Errorf("contextTokens = %v, want nil", *got.ContextTokens)
	}
	if got.Model != "" || got.Title != "" || got.Effort != "" {
		t.Errorf("model=%q title=%q effort=%q, want all empty", got.Model, got.Title, got.Effort)
	}
}

// TestPostToolUseNeverReadsTheTranscript pins the pre-existing boundary that this
// change relies on: only turn/session boundaries ever read the file, so the most
// frequent hook of all stays free.
func TestPostToolUseNeverReadsTheTranscript(t *testing.T) {
	asClaudeCode(t, "s1")
	n := countingParse(t)
	withWatcher(t, false)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("HOME", t.TempDir())
	writeConfig(t, srv.URL)

	payload := `{"session_id":"s1","hook_event_name":"PostToolUse","tool_name":"Bash","transcript_path":"/t.jsonl"}`
	if err := Run("PostToolUse", strings.NewReader(payload)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *n != 0 {
		t.Errorf("PostToolUse read the transcript %d time(s)", *n)
	}
}
