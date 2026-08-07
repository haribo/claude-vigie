package tui

import (
	"strings"
	"testing"
	"time"

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
	}, nil, nil, 100)
	for _, want := range []string{"MACHINE", "minet", "box", "haribo", "3.0M"} {
		if !strings.Contains(out, want) {
			t.Errorf("machines view missing %q:\n%s", want, out)
		}
	}
	if e := renderMachines(nil, nil, nil, 100); !strings.Contains(e, "no sessions") {
		t.Errorf("empty machines missing placeholder: %q", e)
	}
}

func TestWatcherFresh(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if !watcherFresh(now.Add(-5*time.Second).Format(time.RFC3339), now) {
		t.Error("a recent heartbeat should be fresh")
	}
	if watcherFresh(now.Add(-60*time.Second).Format(time.RFC3339), now) {
		t.Error("an old heartbeat should be stale")
	}
	if watcherFresh("", now) {
		t.Error("an empty heartbeat should be stale")
	}
	if watcherFresh("not-a-time", now) {
		t.Error("an unparseable heartbeat should be stale")
	}
}

func TestRenderMachinesFlagsMissingWatcher(t *testing.T) {
	fresh := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	out := renderMachines([]api.SessionView{
		{Machine: "watched", Status: "working", LastSeenAt: "2026-07-29T10:00:00Z"},
		{Machine: "hooks-only", Status: "working", LastSeenAt: "2026-07-29T10:00:00Z"},
	}, map[string]string{"watched": fresh /* "hooks-only" absent → no watcher */}, nil, 120)

	if !strings.Contains(out, "WATCH") {
		t.Errorf("missing WATCH column header:\n%s", out)
	}
	if !strings.Contains(out, "live") {
		t.Errorf("watched machine should show a live watcher:\n%s", out)
	}
	if !strings.Contains(out, "no watcher on hooks-only") {
		t.Errorf("banner should name the machine without a watcher:\n%s", out)
	}
	if strings.Contains(out, "on watched") {
		t.Errorf("machine with a fresh watcher wrongly flagged:\n%s", out)
	}
	if !strings.Contains(out, "vigie watch") {
		t.Errorf("banner should suggest `vigie watch`:\n%s", out)
	}
}

func TestRenderMachinesHealthyFleetNoBanner(t *testing.T) {
	fresh := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	out := renderMachines(
		[]api.SessionView{{Machine: "m1", Status: "working", LastSeenAt: "2026-07-29T10:00:00Z"}},
		map[string]string{"m1": fresh}, nil, 120)
	if strings.Contains(out, "no watcher") {
		t.Errorf("a healthy fleet should show no banner:\n%s", out)
	}
	if !strings.Contains(out, "live") {
		t.Errorf("healthy machine should show live:\n%s", out)
	}
}

// TestRenderMachinesShowsVersion is the #356 display: the Machines tab shows each
// watcher's reported version, and a dash for a machine reporting on hooks alone.
func TestRenderMachinesShowsVersion(t *testing.T) {
	fresh := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	out := renderMachines(
		[]api.SessionView{
			{Machine: "m1", Status: "working", LastSeenAt: "2026-07-29T10:00:00Z"},
			{Machine: "hooks-only", Status: "working", LastSeenAt: "2026-07-29T10:00:00Z"},
		},
		map[string]string{"m1": fresh},
		map[string]api.VersionInfo{"m1": {Version: "0.3.0", Commit: "abc1234"}},
		140)
	if !strings.Contains(out, "VERSION") {
		t.Errorf("missing VERSION column header:\n%s", out)
	}
	if !strings.Contains(out, "0.3.0") {
		t.Errorf("watcher version should be shown:\n%s", out)
	}
	if !strings.Contains(out, "—") {
		t.Errorf("a machine with no watcher version should show a dash:\n%s", out)
	}
}
