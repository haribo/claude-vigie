package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
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

func TestThinkingRendering(t *testing.T) {
	if got := statusCell(api.SessionView{Status: "thinking"}); got != "● thinking" {
		t.Errorf("statusCell = %q, want ● thinking", got)
	}
	out := renderSummary([]api.SessionView{{Status: "thinking"}, {Status: "working"}}, nil)
	if !strings.Contains(out, "● thinking 1") {
		t.Errorf("summary missing thinking count:\n%s", out)
	}
	// A sub-state shown only when present.
	if out := renderSummary([]api.SessionView{{Status: "idle"}}, nil); strings.Contains(out, "thinking") {
		t.Errorf("summary should omit thinking when none:\n%s", out)
	}
}

func TestActivityCell(t *testing.T) {
	if got := detailCell(api.SessionView{Detail: "Edit render.go"}); got != "Edit render.go" {
		t.Errorf("detailCell = %q, want Edit render.go", got)
	}
	if got := detailCell(api.SessionView{}); got != "-" {
		t.Errorf("empty detailCell = %q, want -", got)
	}
}

func TestConnGlyph(t *testing.T) {
	if g := (model{sseLive: true}).connGlyph(); !strings.Contains(g, "●") {
		t.Errorf("live conn = %q, want ●", g)
	}
	if g := (model{sseLive: false, err: errTest}).connGlyph(); !strings.Contains(g, "○") {
		t.Errorf("offline conn = %q, want ○", g)
	}
	if g := (model{}).connGlyph(); !strings.Contains(g, "◍") {
		t.Errorf("reconnecting conn = %q, want ◍", g)
	}
}

var errTest = fmtError("boom")

type fmtError string

func (e fmtError) Error() string { return string(e) }

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
