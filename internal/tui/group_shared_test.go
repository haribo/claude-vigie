package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// #546. The dashboard groups the table too, and it has its own copy of the mode
// names because the browser cannot import Go. A vocabulary copied per consumer is
// what #421, #422 and #466 were, so this reads the shipped `lib.js` and fails on
// any drift — the same guard `TestDashboardSharesTheAttentionSet` gives the
// attention set, and #544 requires wherever the boundary is checkable.
//
// It compares the *names*, in enum order, because those are what an operator's
// saved preference holds on both sides: a mode renamed on one client and not the
// other silently resets that operator's grouping.

// jsArrayFromFile extracts a `const NAME = ["a", "b"]` literal from a JavaScript
// file. A local copy of the helper `internal/status` uses; the two test packages
// do not share fixtures, and importing across `_test` packages to save eighteen
// lines is a worse trade.
func jsArrayFromFile(t *testing.T, path, name string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	m := regexp.MustCompile(`const\s+` + name + `\s*=\s*\[([^\]]*)\]`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("%s: no `const %s = [...]` found — was it renamed?", path, name)
	}
	var out []string
	for _, raw := range strings.Split(m[1], ",") {
		if v := strings.Trim(strings.TrimSpace(raw), `"'`); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func TestDashboardSharesTheGroupModes(t *testing.T) {
	var want []string
	for g := groupNone; g < groupByCount; g++ {
		n, ok := groupNames[g]
		if !ok {
			t.Fatalf("groupNames has no name for mode %d — the enum and the map disagree", g)
		}
		want = append(want, n)
	}

	got := jsArrayFromFile(t, "../../internal/web/static/lib.js", "GROUP_MODES")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("lib.js GROUP_MODES = %v, want %v (internal/tui.groupNames, in enum order)", got, want)
	}
}

// The project key is the last path segment, not the directory: two machines that
// checked the same project out under different roots must land in one group.
func TestGroupKeyUsesTheProjectName(t *testing.T) {
	s := stubSessionForHaystack()
	s.ProjectDir = "/home/ada/dev/api-gateway"
	if got := groupKey(s, groupProject); got != "api-gateway" {
		t.Errorf("project group key = %q, want the last segment", got)
	}
	if got := groupKey(s, groupMachine); got != "orion-dev" {
		t.Errorf("machine group key = %q", got)
	}
}
