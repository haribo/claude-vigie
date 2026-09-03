package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// ADR-0013. Another CLI reads `~/.claude/settings.json` and runs the hooks it
// finds there, so it calls `vigie report` exactly as Claude Code does. Its
// sessions arrived as rows vigie could not name, could not end and counted one
// per subagent — the fleet count stopped meaning anything (#709).
//
// Claude Code exports `CLAUDE_CODE_SESSION_ID` into the hook process, equal to the
// session being reported. Measured, not assumed: a throwaway PostToolUse hook
// dumping its own environment carried it, matching the payload's `session_id`.
//
// The absent case must refuse. A foreign CLI sets nothing, so "absent → allow"
// would be no guard at all.

// asClaudeCode makes the calling test look like a hook Claude Code spawned for
// this session: the environment ADR-0013 requires before a report is posted.
// Tests written before that decision reported from nowhere, which no hook does.
func asClaudeCode(t *testing.T, sessionID string) {
	t.Helper()
	t.Setenv(sessionIDEnv, sessionID)
}

// postSpy returns a server recording what reaches it, and how many times.
func postSpy(t *testing.T) (url string, got *api.ReportRequest, calls *int) {
	t.Helper()
	got, calls = &api.ReportRequest{}, new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		_ = json.NewDecoder(r.Body).Decode(got)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got, calls
}

func TestAReportFromClaudeCodeIsPosted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	url, got, calls := postSpy(t)
	writeConfig(t, url)
	asClaudeCode(t, "s1")

	if err := Run("UserPromptSubmit", strings.NewReader(
		`{"session_id":"s1","cwd":"/p","hook_event_name":"UserPromptSubmit"}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *calls != 1 || got.SessionID != "s1" {
		t.Errorf("calls=%d session=%q; a hook running under Claude Code must report", *calls, got.SessionID)
	}
}

func TestAReportWithNoClaudeCodeSessionIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	url, _, calls := postSpy(t)
	writeConfig(t, url)
	t.Setenv(sessionIDEnv, "") // what another CLI's hook runner leaves behind

	err := Run("UserPromptSubmit", strings.NewReader(
		`{"session_id":"foreign-1","cwd":"/p","hook_event_name":"UserPromptSubmit"}`))
	if err == nil {
		t.Error("no error for a report from outside Claude Code")
	}
	if *calls != 0 {
		t.Errorf("%d report(s) reached the daemon; the fleet count includes sessions vigie cannot name", *calls)
	}
}

// Presence of the variable proves a Claude-shaped environment. Equality proves the
// caller is reporting *its own* session rather than relaying someone else's.
func TestAReportForAnotherSessionIsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	url, _, calls := postSpy(t)
	writeConfig(t, url)
	t.Setenv(sessionIDEnv, "mine")

	err := Run("UserPromptSubmit", strings.NewReader(
		`{"session_id":"someone-else","cwd":"/p","hook_event_name":"UserPromptSubmit"}`))
	if err == nil {
		t.Error("no error for a report about a session this process does not run")
	}
	if *calls != 0 {
		t.Errorf("%d report(s) reached the daemon for another session", *calls)
	}
}

// A refusal must leave a trace. A hook always exits 0, so a silent refusal means
// the day the variable is renamed vigie loses the fleet and says nothing — the
// failure #663 was about, one layer up.
func TestARefusalIsRecordedForThePreflight(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	url, _, _ := postSpy(t)
	writeConfig(t, url)
	t.Setenv(sessionIDEnv, "")

	if n, _ := RefusedReports(); n != 0 {
		t.Fatalf("start with %d refusals, want 0", n)
	}
	for i := 0; i < 3; i++ {
		_ = Run("PostToolUse", strings.NewReader(`{"session_id":"foreign-1","hook_event_name":"PostToolUse"}`))
	}
	n, last := RefusedReports()
	if n != 3 {
		t.Errorf("recorded %d refusals, want 3 — the operator is told a number, not a fact", n)
	}
	if last.IsZero() {
		t.Error("no time recorded; `when` is what tells a current problem from an old one")
	}
}
