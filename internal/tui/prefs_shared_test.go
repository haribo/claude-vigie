package tui

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// Hiding a session that has gone quiet is a preference, so the rule stays in each
// client (ADR-0011) and the dashboard holds a second implementation. Both read
// this case list (#547, #619).
//
// The steps are checked alongside the behaviour because they are what an operator's
// saved value is one of: a preset offered on one client and not the other leaves a
// stored threshold the dashboard cannot represent.

type idleFixture struct {
	PresetsSeconds []int64 `json:"presets_seconds"`
	Cases          []struct {
		Why          string `json:"why"`
		Now          string `json:"now"`
		Seen         string `json:"seen"`
		AfterSeconds int64  `json:"after_seconds"`
		Status       string `json:"status"`
		Hidden       bool   `json:"hidden"`
	} `json:"cases"`
}

func loadIdleFixture(t *testing.T) idleFixture {
	t.Helper()
	f := loadFixture[idleFixture](t, "idle-cases.json")
	if len(f.PresetsSeconds) == 0 || len(f.Cases) == 0 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	return f
}

func TestTheIdlePresetsAgreeWithTheSharedFixture(t *testing.T) {
	f := loadIdleFixture(t)
	if len(idlePresets) != len(f.PresetsSeconds) {
		t.Fatalf("idlePresets = %v, the shared fixture says %v seconds", idlePresets, f.PresetsSeconds)
	}
	for i, want := range f.PresetsSeconds {
		if got := int64(idlePresets[i] / time.Second); got != want {
			t.Errorf("idlePresets[%d] = %ds, the shared fixture says %ds", i, got, want)
		}
	}
}

func TestIdleHidingAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadIdleFixture(t).Cases {
		now, err := time.Parse(time.RFC3339, c.Now)
		if err != nil {
			t.Fatalf("the fixture's `now` is not a timestamp: %q", c.Now)
		}
		p := prefs{idleHideAfter: time.Duration(c.AfterSeconds) * time.Second}
		status := c.Status
		if status == "" {
			status = "idle"
		}
		hidden := !p.visible(api.SessionView{Status: status, LastSeenAt: c.Seen}, now)
		if hidden != c.Hidden {
			t.Errorf("hidden(seen=%q, after=%ds) = %v, want %v — %s",
				c.Seen, c.AfterSeconds, hidden, c.Hidden, c.Why)
		}
	}
}
