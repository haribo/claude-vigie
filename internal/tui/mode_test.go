package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestPermissionModeLabel(t *testing.T) {
	cases := map[string]string{
		"default":           "manual",
		"acceptEdits":       "accept",
		"plan":              "plan",
		"auto":              "auto",
		"bypassPermissions": "bypass",
		"":                  "-",
		"someNewMode":       "someNewMode", // unknown surfaced raw, never faked as "manual"
	}
	for raw, want := range cases {
		if got, _ := permissionModeLabel(raw); got != want {
			t.Errorf("permissionModeLabel(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestModeCell(t *testing.T) {
	if got := modeCell(api.SessionView{PermissionMode: "bypassPermissions"}); got != "bypass" {
		t.Errorf("modeCell = %q, want bypass", got)
	}
	if got := modeCell(api.SessionView{}); got != "-" {
		t.Errorf("modeCell(empty) = %q, want -", got)
	}
	if got := modeDetail(api.SessionView{PermissionMode: "plan"}); got != "plan — awaiting plan approval" {
		t.Errorf("modeDetail(plan) = %q", got)
	}
}
