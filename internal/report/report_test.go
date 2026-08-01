package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
)

func writeConfig(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(&config.Config{ServerURL: serverURL, Token: "tok", Machine: "laptop"}); err != nil {
		t.Fatalf("save config: %v", err)
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
	content := `{"type":"custom-title","customTitle":"my-conv"}` + "\n" +
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","usage":{"input_tokens":500,"output_tokens":120}}}` + "\n"
	if err := os.WriteFile(transcript, []byte(content), 0o600); err != nil {
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
	if got.Title != "my-conv" {
		t.Errorf("title = %q, want my-conv", got.Title)
	}
}

func TestRunMissingSessionID(t *testing.T) {
	if err := Run("Stop", strings.NewReader(`{"cwd":"/p"}`)); err == nil {
		t.Fatal("expected an error for missing session_id, got nil")
	}
}

func TestHookActivity(t *testing.T) {
	cases := []struct {
		name  string
		event string
		p     hookPayload
		want  string
	}{
		// #236 — an idle_prompt lands the session on idle, so it carries no "doing".
		{"idle_prompt carries none", "Notification",
			hookPayload{NotificationType: "idle_prompt", Message: "waiting for your input"}, ""},
		{"permission_prompt keeps its message", "Notification",
			hookPayload{NotificationType: "permission_prompt", Message: "allow Bash?"}, "allow Bash?"},
		{"bare notification keeps its message", "Notification",
			hookPayload{Message: "heads up"}, "heads up"},
		{"other events carry none", "Stop", hookPayload{}, ""},
	}
	for _, c := range cases {
		if got := hookActivity(c.event, c.p); got != c.want {
			t.Errorf("%s: hookActivity(%q, %+v) = %q, want %q", c.name, c.event, c.p, got, c.want)
		}
	}
}
