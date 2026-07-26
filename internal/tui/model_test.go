package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
