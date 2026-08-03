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

func TestShortModel(t *testing.T) {
	if got := shortModel("claude-opus-4-8"); got != "opus-4-8" {
		t.Errorf("shortModel = %q, want opus-4-8", got)
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

func TestShortIDAndProjectName(t *testing.T) {
	if got := shortID("5c483c16-96b5-4f61"); got != "5c483c16" {
		t.Errorf("shortID = %q, want 5c483c16", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID(short) = %q, want abc", got)
	}
	if got := projectName("/home/x/claude-fleet"); got != "claude-fleet" {
		t.Errorf("projectName = %q, want claude-fleet", got)
	}
	if got := projectName(""); got != "-" {
		t.Errorf("projectName(empty) = %q, want -", got)
	}
}

func TestSessionName(t *testing.T) {
	if got := sessionName(api.SessionView{Title: "my-conv", ID: "5c483c16-x"}); got != "my-conv" {
		t.Errorf("sessionName with title = %q, want my-conv", got)
	}
	if got := sessionName(api.SessionView{ID: "5c483c16-x"}); got != "5c483c16" {
		t.Errorf("sessionName without title = %q, want short id 5c483c16", got)
	}
}

func TestRenderTableWide(t *testing.T) {
	out := renderTable([]api.SessionView{{
		ID: "5c483c16-96b5-4f61", Title: "my-session", Machine: "laptop",
		ProjectDir: "/home/x/proj", GitBranch: "main",
		Model: "claude-opus-4-8", Status: "working",
		Usage:      api.Usage{OutputTokens: 1500, InputTokens: 500},
		LastSeenAt: "2026-07-26T17:01:32Z",
	}}, 200, -1, sortState{})
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
	if got := orDash(shortModel("")); got != "-" {
		t.Errorf("empty model cell = %q, want %q", got, "-")
	}
	if got := orDash(shortModel("claude-opus-4-8")); got != "opus-4-8" {
		t.Errorf("known model cell = %q, want opus-4-8", got)
	}
}

func TestRenderTableNarrowHidesColumns(t *testing.T) {
	out := renderTable([]api.SessionView{{
		ID: "5c483c16", Title: "my-session", Machine: "laptop",
		ProjectDir: "/home/x/proj", GitBranch: "main", Status: "working",
		LastSeenAt: "2026-07-26T17:01:32Z",
	}}, 60, -1, sortState{})
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

func TestRCCellAndSummary(t *testing.T) {
	if got := rcCell(api.SessionView{RemoteControl: true}); got != "◉" {
		t.Errorf("rc on = %q, want ◉", got)
	}
	if got := rcCell(api.SessionView{RemoteControl: false}); got != "○" {
		t.Errorf("rc off = %q, want ○", got)
	}
	out := renderSummary([]api.SessionView{
		{Status: "working", RemoteControl: true},
		{Status: "idle", RemoteControl: true},
		{Status: "idle"},
	}, nil)
	if !strings.Contains(out, "rc ") || !strings.Contains(out, "◉ 2") {
		t.Errorf("summary missing rc counter (want ◉ 2):\n%s", out)
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
