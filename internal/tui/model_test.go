package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestRenderTabBar(t *testing.T) {
	out := renderTabBar(tabUsage)
	for _, want := range []string{"Sessions", "Usage", "Machines"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar missing %q: %s", want, out)
		}
	}
	if !strings.Contains(out, "[2 Usage]") {
		t.Errorf("active tab not highlighted: %s", out)
	}
}

func TestTabSwitching(t *testing.T) {
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	var m tea.Model = model{}

	m, _ = m.Update(key("2"))
	if m.(model).tab != tabUsage {
		t.Errorf("after '2': tab = %d, want tabUsage", m.(model).tab)
	}
	m, _ = m.Update(key("3"))
	if m.(model).tab != tabMachines {
		t.Errorf("after '3': tab = %d, want tabMachines", m.(model).tab)
	}
	m, _ = m.Update(key("1"))
	if m.(model).tab != tabSessions {
		t.Errorf("after '1': tab = %d, want tabSessions", m.(model).tab)
	}
}

func TestViewHasTabBar(t *testing.T) {
	out := model{}.View()
	for _, want := range []string{"Sessions", "Usage", "Machines"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing tab %q", want)
		}
	}
}

func TestCursorAndDetail(t *testing.T) {
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	named := func(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }
	var m tea.Model = model{sessions: []api.SessionView{{ID: "a"}, {ID: "b"}, {ID: "c"}}}

	m, _ = m.Update(key("j")) // down
	m, _ = m.Update(key("j"))
	m, _ = m.Update(key("j")) // clamp at last (index 2)
	if m.(model).cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.(model).cursor)
	}
	m, _ = m.Update(key("k")) // up
	if m.(model).cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.(model).cursor)
	}

	m, _ = m.Update(named(tea.KeyEnter))
	if !m.(model).detail {
		t.Error("enter did not open detail")
	}
	m, _ = m.Update(named(tea.KeyEsc))
	if m.(model).detail {
		t.Error("esc did not close detail")
	}
}

func TestRenderDetailContainsFields(t *testing.T) {
	out := renderDetail(api.SessionView{
		ID: "5c483c16-x", Title: "claude-fleet", Machine: "minet",
		ProjectDir: "/home/haribo/dev/claude-fleet", GitBranch: "develop",
		Model: "claude-opus-4-8", Status: "working", LastTool: "Bash",
		Usage:     api.Usage{InputTokens: 100, OutputTokens: 200},
		StartedAt: "2026-07-26T10:00:00Z", LastSeenAt: "2026-07-26T10:05:00Z",
	})
	for _, want := range []string{"claude-fleet", "5c483c16-x", "minet", "/home/haribo/dev/claude-fleet", "develop", "Bash", "Started", "Output"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("empty history = %q, want empty", got)
	}
	r := []rune(sparkline([]int{0, 1, 2, 4}))
	if len(r) != 4 {
		t.Fatalf("sparkline length = %d, want 4", len(r))
	}
	if r[0] != '▁' {
		t.Errorf("zero block = %q, want ▁", string(r[0]))
	}
	if r[3] != '█' {
		t.Errorf("max block = %q, want █", string(r[3]))
	}
}

func TestRenderSummary(t *testing.T) {
	sessions := []api.SessionView{
		{Status: "working", Usage: api.Usage{OutputTokens: 1000}},
		{Status: "working", Usage: api.Usage{OutputTokens: 500}},
		{Status: "idle", Usage: api.Usage{OutputTokens: 200}},
	}
	out := renderSummary(sessions, []int{1, 2, 2})
	for _, want := range []string{"working 2", "idle 1", "waiting 0", "out ", "activity "} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}
