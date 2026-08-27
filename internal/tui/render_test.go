package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestHumanizeTokens(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1500: "1.5k", 2_500_000: "2.5M"}
	for n, want := range cases {
		if got := humanizeTokens(n); got != want {
			t.Errorf("humanizeTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestColumnWidthsFitContent guards the #319 tightening: no column is narrower
// than its header (plus the sort arrow, if sortable), and the resnugged
// fixed-format columns still hold their widest real cell.
func TestColumnWidthsFitContent(t *testing.T) {
	sortable := map[string]bool{}
	for _, h := range sortColumn {
		sortable[h] = true
	}
	byHeader := map[string]int{}
	for _, c := range columns {
		byHeader[c.header] = c.width
		need := len([]rune(c.header))
		if sortable[c.header] {
			need++ // the header carries a 1-rune sort arrow when it is the active key
		}
		if c.width < need {
			t.Errorf("%s: width %d < header need %d", c.header, c.width, need)
		}
	}

	// widest humanizeTokens output over the whole range is "999.9k"/"999.9M" (6).
	maxTok := 0
	for _, n := range []int64{0, 999, 9_999, 999_900, 9_999_000, 999_900_000} {
		if l := len(humanizeTokens(n)); l > maxTok {
			maxTok = l
		}
	}
	for h, need := range map[string]int{"OUT": maxTok, "TOTAL": maxTok, "EFFORT": len("medium")} {
		if byHeader[h] < need {
			t.Errorf("%s: width %d < content need %d", h, byHeader[h], need)
		}
	}
}

func TestPadAndTruncate(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q, want %q", got, "ab   ")
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q, want %q", got, "abc…")
	}
}

func TestRenderTableWide(t *testing.T) {
	// Name, Project and ModelShort come from the daemon now (ADR-0011, #618); the
	// raw fields beside them are what it derived them from.
	out := renderTable([]api.SessionView{{
		ID: "5c483c16-96b5-4f61", Title: "my-session", Name: "my-session", Machine: "laptop",
		ProjectDir: "/home/x/proj", Project: "proj", GitBranch: "main",
		Model: "claude-opus-4-8", ModelShort: "opus-4-8", Status: "working",
		Usage:      api.Usage{OutputTokens: 1500, InputTokens: 500},
		LastSeenAt: "2026-07-26T17:01:32Z",
	}}, columns, 200, -1, sortState{})
	// SEEN is relative (time.Now()) and SESSION moved to the detail panel, so
	// neither is asserted here.
	for _, want := range []string{"NAME", "my-session", "DIR", "proj", "main", "opus-4-8", "1.5k", "STATUS", "working"} {
		if !strings.Contains(out, want) {
			t.Errorf("wide table missing %q:\n%s", want, out)
		}
	}
}

func TestModelCellUnknownShowsDash(t *testing.T) {
	// A session open before Claude's first reply has no model name anywhere; the
	// MODEL cell must read as a dash, not an ambiguous blank (#242) — while a
	// known model still renders its short form.
	cell := columnByKey()["model"].cell
	if got := cell(api.SessionView{}); got != "-" {
		t.Errorf("empty model cell = %q, want %q", got, "-")
	}
	if got := cell(api.SessionView{Model: "claude-opus-4-8", ModelShort: "opus-4-8"}); got != "opus-4-8" {
		t.Errorf("known model cell = %q, want opus-4-8", got)
	}
}

func TestRenderTableNarrowHidesColumns(t *testing.T) {
	out := renderTable([]api.SessionView{{
		ID: "5c483c16", Title: "my-session", Machine: "laptop",
		ProjectDir: "/home/x/proj", GitBranch: "main", Status: "working",
		LastSeenAt: "2026-07-26T17:01:32Z",
	}}, columns, 60, -1, sortState{})
	for _, want := range []string{"NAME", "DIR", "STATUS"} {
		if !strings.Contains(out, want) {
			t.Errorf("narrow table dropped mandatory column %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"SESSION", "BRANCH", "TOTAL"} {
		if strings.Contains(out, gone) {
			t.Errorf("narrow table still shows %q (should be hidden):\n%s", gone, out)
		}
	}
}

func TestRCCell(t *testing.T) {
	if got := rcCell(api.SessionView{RemoteControl: true}); got != "◉" {
		t.Errorf("rc on = %q, want ◉", got)
	}
	if got := rcCell(api.SessionView{RemoteControl: false}); got != "○" {
		t.Errorf("rc off = %q, want ○", got)
	}
}

func TestRenderDetailUserAndRC(t *testing.T) {
	out := renderDetail(api.SessionView{
		ID: "s1", Title: "t", User: "alice", Machine: "m",
		Status: "working", RemoteControl: true,
	})
	for _, want := range []string{"User", "alice", "Remote control", "on"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
}
