package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestAggregateMachines(t *testing.T) {
	m := aggregateMachines([]api.SessionView{
		{Machine: "a", Status: "working", Usage: api.Usage{OutputTokens: 100}},
		{Machine: "a", Status: "idle", Usage: api.Usage{OutputTokens: 200}},
		{Machine: "b", Status: "waiting"},
	})
	if len(m) != 2 || m[0].name != "a" || m[0].sessions != 2 {
		t.Fatalf("aggregate = %+v, want a(2) first", m)
	}
	if m[0].working != 1 || m[0].idle != 1 || m[0].out != 300 {
		t.Errorf("machine a: %+v", m[0])
	}
	if m[1].name != "b" || m[1].waiting != 1 {
		t.Errorf("machine b: %+v", m[1])
	}
}

func TestRenderMachines(t *testing.T) {
	out := renderMachines([]api.SessionView{
		{Machine: "minet", Status: "working", User: "haribo", Usage: api.Usage{OutputTokens: 2000000}, LastSeenAt: "2026-07-29T10:00:00Z"},
		{Machine: "minet", Status: "idle", User: "haribo", Usage: api.Usage{OutputTokens: 1000000}, LastSeenAt: "2026-07-29T10:01:00Z"},
		{Machine: "box", Status: "waiting", User: "bob", Usage: api.Usage{OutputTokens: 500000}, LastSeenAt: "2026-07-29T09:00:00Z"},
	}, 100)
	for _, want := range []string{"MACHINE", "minet", "box", "haribo", "3.0M"} {
		if !strings.Contains(out, want) {
			t.Errorf("machines view missing %q:\n%s", want, out)
		}
	}
	if e := renderMachines(nil, 100); !strings.Contains(e, "no sessions") {
		t.Errorf("empty machines missing placeholder: %q", e)
	}
}
