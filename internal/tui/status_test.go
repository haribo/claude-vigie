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

// TestDetailCellPrecedence: DETAIL was already taken by the watcher's activity,
// so the cell needs a stated order rather than whichever branch happens to run
// first. A raised call is why the row is blinking and outranks everything; an
// API error outranks the activity, because once the API answers 529 the tool
// that ran last is of no interest (#584).
func TestDetailCellPrecedence(t *testing.T) {
	for _, c := range []struct {
		name string
		s    api.SessionView
		want string
	}{
		{"a call outranks an API error",
			api.SessionView{Status: "error", APIErrorStatus: 529, Detail: "Bash", CallAt: "2026-08-19T10:00:00Z", CallMessage: "done"},
			"done"},
		{"an API error outranks the activity",
			api.SessionView{Status: "error", APIErrorStatus: 529, Detail: "Bash"}, "529 Overloaded"},
		{"an unknown code is shown bare",
			api.SessionView{Status: "error", APIErrorStatus: 503, Detail: "Bash"}, "503"},
		{"error with no code keeps the activity",
			api.SessionView{Status: "error", Detail: "Bash"}, "Bash"},
		{"a code on a non-error status is not shown",
			api.SessionView{Status: "working", APIErrorStatus: 529, Detail: "Bash"}, "Bash"},
		{"nothing to show is a dash",
			api.SessionView{Status: "error"}, "-"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := detailCell(c.s); got != c.want {
				t.Errorf("detailCell = %q, want %q", got, c.want)
			}
		})
	}
}

func TestThinkingRendering(t *testing.T) {
	if got := statusCell(api.SessionView{Status: "thinking"}); got != "● thinking" {
		t.Errorf("statusCell = %q, want ● thinking", got)
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
