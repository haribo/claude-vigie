package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #584 moved the HTTP code out of this cell. It was the only status carrying a
// refinement inside its own cell, against the convention
// design/session-status.md § 2 states for every other one.
func TestStatusCell(t *testing.T) {
	if got := statusCell(api.SessionView{Status: "error", APIErrorStatus: 529}); got != "● error" {
		t.Errorf("statusCell = %q, want ● error — the code belongs in DETAIL now", got)
	}
	if got := statusCell(api.SessionView{Status: "error"}); got != "● error" {
		t.Errorf("statusCell (no code) = %q, want ● error", got)
	}
	if got := statusCell(api.SessionView{Status: "working"}); got != "● working" {
		t.Errorf("statusCell = %q, want ● working", got)
	}
}

func TestThinkingRendering(t *testing.T) {
	if got := statusCell(api.SessionView{Status: "thinking"}); got != "● thinking" {
		t.Errorf("statusCell = %q, want ● thinking", got)
	}
}

// The precedence behind DETAIL — call, then API error, then activity, then a dash
// — is the daemon's and is proved in internal/server/naming_test.go. What this
// client owes is to render the answer it was given rather than recompute one.
func TestTheDetailCellRendersWhatTheDaemonDerived(t *testing.T) {
	cell := columnByKey()["detail"].cell
	if got := cell(api.SessionView{DetailText: "Edit render.go", Detail: "ignored"}); got != "Edit render.go" {
		t.Errorf("detail cell = %q, want the derived text", got)
	}
	if got := cell(api.SessionView{DetailText: "-"}); got != "-" {
		t.Errorf("detail cell = %q, want -", got)
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
