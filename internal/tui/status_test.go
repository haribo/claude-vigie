package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestStatusCell(t *testing.T) {
	if got := statusCell(api.SessionView{Status: "error", APIErrorStatus: 529}); got != "● error 529" {
		t.Errorf("statusCell = %q, want ● error 529", got)
	}
	if got := statusCell(api.SessionView{Status: "error"}); got != "● error" {
		t.Errorf("statusCell (no code) = %q, want ● error", got)
	}
	if got := statusCell(api.SessionView{Status: "working"}); got != "● working" {
		t.Errorf("statusCell = %q, want ● working", got)
	}
}

func TestStatusDetailAndLabel(t *testing.T) {
	if got := statusDetail(api.SessionView{Status: "error", APIErrorStatus: 529}); got != "error — 529 Overloaded" {
		t.Errorf("statusDetail = %q, want error — 529 Overloaded", got)
	}
	if got := statusDetail(api.SessionView{Status: "idle"}); got != "idle" {
		t.Errorf("statusDetail = %q, want idle", got)
	}
	for code, want := range map[int]string{
		429: "429 Rate limited", 500: "500 Internal server error",
		529: "529 Overloaded", 418: "418",
	} {
		if got := apiErrorLabel(code); got != want {
			t.Errorf("apiErrorLabel(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestRenderSummaryErrorCount(t *testing.T) {
	out := renderSummary([]api.SessionView{
		{Status: "working"}, {Status: "error", APIErrorStatus: 500}, {Status: "idle"},
	}, nil)
	if !strings.Contains(out, "● error 1") {
		t.Errorf("summary missing error count:\n%s", out)
	}
	// No errored sessions → no error segment (it is an alert, shown only when present).
	if out := renderSummary([]api.SessionView{{Status: "idle"}}, nil); strings.Contains(out, "error") {
		t.Errorf("summary should omit error count when none:\n%s", out)
	}
}
