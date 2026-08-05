package tui

import (
	"strings"
	"testing"
	"time"
)

func TestLabels(t *testing.T) {
	if !strings.Contains(retentionLabel(0), "off") {
		t.Error("retentionLabel(0) should read off")
	}
	if got := retentionLabel(48 * time.Hour); got == "" || strings.Contains(got, "off") {
		t.Errorf("retentionLabel(48h) = %q, want a duration", got)
	}
	if !strings.Contains(onOffLabel(true), "on") || !strings.Contains(onOffLabel(false), "off") {
		t.Error("onOffLabel should read on / off")
	}
	if !strings.Contains(idleLabel(0), "never") {
		t.Error("idleLabel(0) should read never")
	}
}

func TestJoinLR(t *testing.T) {
	// Wide enough → left and right on one line with padding between.
	out := joinLR("LEFT", "RIGHT", 40)
	if !strings.Contains(out, "LEFT") || !strings.Contains(out, "RIGHT") {
		t.Errorf("joinLR dropped a side: %q", out)
	}
	// Too narrow → clamps to width (no overflow), keeps a prefix of the primary
	// left side and drops the secondary right (#328).
	if got := joinLR("LEFT", "RIGHT", 3); len([]rune(got)) > 3 || !strings.HasPrefix("LEFT", got) {
		t.Errorf("joinLR narrow = %q", got)
	}
}

func TestSummaryRightAndFilterLine(t *testing.T) {
	m := model{sortKey: sortTokens, groupBy: groupMachine, sseLive: true, clock: fixedClock}
	right := m.summaryRight()
	if !strings.Contains(right, "sort") || !strings.Contains(right, sortNames[sortTokens]) || !strings.Contains(right, "group") {
		t.Errorf("summaryRight = %q", right)
	}
	fl := model{filter: "auth", filtering: true}.filterLine()
	if !strings.Contains(fl, "auth") || !strings.Contains(fl, "▌") {
		t.Errorf("filterLine = %q", fl)
	}
}

func TestRenderSettings(t *testing.T) {
	m := model{prefs: defaultPrefs(), serverRetention: 720 * time.Hour, clock: fixedClock}
	out := m.renderSettings()
	for _, want := range []string{"Hide ended sessions", "Hide idle after", "Session retention", "Desktop notifications"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSettings missing %q:\n%s", want, out)
		}
	}
}

func fixedClock() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }
