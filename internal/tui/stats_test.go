package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haribo/claude-fleet/internal/api"
)

func TestBucketStats(t *testing.T) {
	daily := []api.DailyStat{
		{Day: "2026-07-27", Model: "opus", OutputTokens: 100, WaitingSeconds: 60},
		{Day: "2026-07-28", Model: "opus", OutputTokens: 200, WaitingSeconds: 30},
		{Day: "2026-07-28", Model: "sonnet", OutputTokens: 50},
	}

	day := bucketStats(daily, periodDay)
	if len(day) != 2 {
		t.Fatalf("day buckets = %d, want 2", len(day))
	}
	if day[1].total != 250 {
		t.Errorf("2026-07-28 total = %d, want 250", day[1].total)
	}
	if day[1].tokens["sonnet"] != 50 {
		t.Errorf("2026-07-28 sonnet = %d, want 50", day[1].tokens["sonnet"])
	}

	mon := bucketStats(daily, periodMonth)
	if len(mon) != 1 || mon[0].total != 350 {
		t.Fatalf("month buckets = %+v, want one totalling 350", mon)
	}

	tot := bucketStats(daily, periodTotal)
	if len(tot) != 1 || tot[0].total != 350 || tot[0].waiting != 90 {
		t.Errorf("total bucket = %+v, want total 350 waiting 90", tot)
	}
}

func TestStatsPeriodKeys(t *testing.T) {
	var m tea.Model = model{tab: tabStats}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if m.(model).statsPeriod != periodMonth {
		t.Errorf("'m' → %v, want month", m.(model).statsPeriod)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.(model).statsPeriod != periodTotal {
		t.Errorf("'t' → %v, want total", m.(model).statsPeriod)
	}
}

func TestRenderStats(t *testing.T) {
	m := model{
		tab:         tabStats,
		statsPeriod: periodDay,
		stats: api.StatsResponse{
			SessionCount: 2,
			Daily: []api.DailyStat{
				{Day: "2026-07-29", Model: "opus", OutputTokens: 1200, WaitingSeconds: 312, WorkingSeconds: 1660},
			},
			TopSessions: []api.TopSession{
				{Name: "claude-fleet", Machine: "minet", Model: "opus", Status: "waiting", OutputTokens: 1200},
			},
		},
	}
	out := m.renderStats()
	for _, want := range []string{"BOTTLENECK", "TOKENS", "WHERE TIME WENT", "TOP SESSIONS", "claude-fleet"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderStats output missing %q", want)
		}
	}

	if empty := (model{tab: tabStats}).renderStats(); !strings.Contains(empty, "No activity") {
		t.Errorf("empty stats view missing placeholder, got %q", empty)
	}
}
