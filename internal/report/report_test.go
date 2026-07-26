package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/config"
)

func writeConfig(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(&config.Config{ServerURL: serverURL, Token: "tok", Machine: "laptop"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func TestAggregateUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user"}}`,
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":40,"cache_read_input_tokens":10}}}`,
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":40}}}`, // duplicate id: ignored
		`{"type":"assistant","message":{"id":"m2","model":"claude-opus-4-8","usage":{"input_tokens":200,"output_tokens":60,"cache_creation_input_tokens":5}}}`,
		`not json`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	u, model, err := aggregateUsage(path)
	if err != nil {
		t.Fatalf("aggregateUsage: %v", err)
	}
	if u.InputTokens != 300 || u.OutputTokens != 100 || u.CacheReadTokens != 10 || u.CacheCreationTokens != 5 {
		t.Errorf("usage = %+v, want in=300 out=100 cacheRead=10 cacheCreate=5", *u)
	}
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q", model)
	}
}

func TestRunPostsReport(t *testing.T) {
	var got api.ReportRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	writeConfig(t, srv.URL)

	payload := `{"session_id":"s1","cwd":"/p","hook_event_name":"UserPromptSubmit"}`
	if err := Run("UserPromptSubmit", strings.NewReader(payload)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.SessionID != "s1" || got.Event != "UserPromptSubmit" || got.Machine != "laptop" || got.ProjectDir != "/p" {
		t.Errorf("server got %+v", got)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", gotAuth)
	}
	if got.Timestamp == "" {
		t.Error("timestamp is empty")
	}
}

func TestRunStopSendsAggregatedUsage(t *testing.T) {
	var got api.ReportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	writeConfig(t, srv.URL)

	transcript := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(transcript, []byte(
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","usage":{"input_tokens":500,"output_tokens":120}}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	payload := `{"session_id":"s1","cwd":"/p","hook_event_name":"Stop","transcript_path":"` + transcript + `"}`
	if err := Run("Stop", strings.NewReader(payload)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Usage == nil {
		t.Fatal("usage is nil, want aggregated usage")
	}
	if got.Usage.InputTokens != 500 || got.Usage.OutputTokens != 120 {
		t.Errorf("usage = %+v, want in=500 out=120", *got.Usage)
	}
	if got.Model != "claude-opus-4-8" {
		t.Errorf("model = %q", got.Model)
	}
}

func TestRunMissingSessionID(t *testing.T) {
	if err := Run("Stop", strings.NewReader(`{"cwd":"/p"}`)); err == nil {
		t.Fatal("expected an error for missing session_id, got nil")
	}
}
