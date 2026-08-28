package tui

import (
	"fmt"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// The labels moved to the daemon with the rest of the naming family (ADR-0011,
// #618) and are proved in internal/server/naming_test.go, against the same shared
// fixture the dashboard reads. What is left here is the color, and the one rule
// that matters about it: a mode this build has never heard of gets no rung on the
// vigilance scale (#304).
func TestModeColorsRiseWithVigilanceAndStopAtTheUnknown(t *testing.T) {
	distinct := map[string]bool{}
	for _, raw := range []string{"default", "acceptEdits", "plan", "auto", "bypassPermissions"} {
		c := modeStyle(api.SessionView{PermissionMode: raw}).GetForeground()
		if c == nil {
			t.Fatalf("modeStyle(%q) has no color", raw)
		}
		key := fmt.Sprintf("%v", c)
		if distinct[key] {
			t.Errorf("modeStyle(%q) reuses a color — the scale stops separating modes", raw)
		}
		distinct[key] = true
	}
	unknown := modeStyle(api.SessionView{PermissionMode: "someNewMode"})
	if unknown.GetForeground() != dimStyle.GetForeground() {
		t.Error("an unrecognized mode was given a color — the cell already says it is unknown, " +
			"and a color would claim it a place on the scale")
	}
	if none := modeStyle(api.SessionView{}); none.GetForeground() != dimStyle.GetForeground() {
		t.Error("a session with no mode reported should be dim")
	}
}
