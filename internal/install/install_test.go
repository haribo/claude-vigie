package install

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func countOurs(ms []hookMatcher) int {
	n := 0
	for _, m := range ms {
		if matcherIsOurs(m) {
			n++
		}
	}
	return n
}

func TestMergeHooksAddIdempotent(t *testing.T) {
	events := []string{"SessionStart", "Stop"}

	first, err := mergeHooks(nil, events, "/bin/claude-fleet", 5)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, err := mergeHooks(first, events, "/bin/claude-fleet", 5)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("merge not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}

	_, hooks, err := parseSettings(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if got := countOurs(hooks[ev]); got != 1 {
			t.Errorf("event %s has %d of our matchers, want 1", ev, got)
		}
	}
}

func TestMergeHooksPreservesForeign(t *testing.T) {
	existing := []byte(`{
		"model": "opus",
		"hooks": {
			"SessionStart": [{"matcher":"","hooks":[{"type":"command","command":"/usr/bin/other-tool"}]}],
			"PreToolUse": [{"matcher":"Bash","hooks":[{"type":"command","command":"/usr/bin/guard"}]}]
		}
	}`)

	out, err := mergeHooks(existing, []string{"SessionStart"}, "/bin/claude-fleet", 5)
	if err != nil {
		t.Fatal(err)
	}
	s, hooks, err := parseSettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s["model"]; !ok {
		t.Error("lost unrelated top-level key 'model'")
	}
	if len(hooks["PreToolUse"]) != 1 {
		t.Error("lost foreign PreToolUse hook")
	}
	if len(hooks["SessionStart"]) != 2 {
		t.Errorf("SessionStart matchers = %d, want 2 (foreign + ours)", len(hooks["SessionStart"]))
	}
	if countOurs(hooks["SessionStart"]) != 1 {
		t.Error("our SessionStart hook missing")
	}
}

func TestRemoveHooks(t *testing.T) {
	base := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/usr/bin/guard"}]}]}}`)
	merged, err := mergeHooks(base, []string{"SessionStart", "Stop"}, "/bin/claude-fleet", 5)
	if err != nil {
		t.Fatal(err)
	}

	cleaned, err := removeHooks(merged)
	if err != nil {
		t.Fatal(err)
	}
	_, hooks, err := parseSettings(cleaned)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hooks["SessionStart"]; ok {
		t.Error("SessionStart not removed")
	}
	if _, ok := hooks["Stop"]; ok {
		t.Error("Stop not removed")
	}
	if len(hooks["PreToolUse"]) != 1 {
		t.Error("foreign PreToolUse should remain")
	}
}

func TestInstallUninstallRoundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := Install([]string{"Stop"}, "/bin/claude-fleet", 5)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "/bin/claude-fleet report --event=Stop") {
		t.Errorf("settings missing our hook:\n%s", data)
	}

	if _, err := Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after uninstall: %v", err)
	}
	if strings.Contains(string(data), "claude-fleet report") {
		t.Errorf("hook not removed:\n%s", data)
	}
}
