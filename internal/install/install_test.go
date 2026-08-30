package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func countLeg(ms []json.RawMessage, configPath string) int {
	n := 0
	for _, m := range ms {
		if matcherHoldsLeg(m, configPath) {
			n++
		}
	}
	return n
}

func TestMergeHooksAddIdempotent(t *testing.T) {
	events := []string{"SessionStart", "Stop"}

	first, err := mergeHooks(nil, events, "/bin/claude-fleet", "", 5)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, err := mergeHooks(first, events, "/bin/claude-fleet", "", 5)
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
		if got := countLeg(hooks[ev], ""); got != 1 {
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

	out, err := mergeHooks(existing, []string{"SessionStart"}, "/bin/claude-fleet", "", 5)
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
	if countLeg(hooks["SessionStart"], "") != 1 {
		t.Error("our SessionStart hook missing")
	}
}

func TestRemoveHooks(t *testing.T) {
	base := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/usr/bin/guard"}]}]}}`)
	merged, err := mergeHooks(base, []string{"SessionStart", "Stop"}, "/bin/claude-fleet", "", 5)
	if err != nil {
		t.Fatal(err)
	}

	cleaned, err := removeHooks(merged, "")
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

// TestDualLegs installs a production leg and a dev leg (distinct VIGIE_CONFIG)
// side by side, and checks that removing one leaves the other intact.
func TestDualLegs(t *testing.T) {
	events := []string{"Notification"}

	out, err := mergeHooks(nil, events, "/bin/claude-fleet", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	out, err = mergeHooks(out, events, "/dev/bin/claude-fleet", "/tmp/dev.toml", 5)
	if err != nil {
		t.Fatal(err)
	}

	_, hooks, err := parseSettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(hooks["Notification"]); got != 2 {
		t.Fatalf("Notification matchers = %d, want 2 (prod + dev)", got)
	}
	if countLeg(hooks["Notification"], "") != 1 {
		t.Error("production leg missing")
	}
	if countLeg(hooks["Notification"], "/tmp/dev.toml") != 1 {
		t.Error("dev leg missing")
	}
	if !strings.Contains(string(out), "VIGIE_CONFIG="+shellQuote("/tmp/dev.toml")) { // quoted since #513
		t.Errorf("dev leg command missing VIGIE_CONFIG:\n%s", out)
	}

	// Removing the dev leg leaves production intact.
	out, err = removeHooks(out, "/tmp/dev.toml")
	if err != nil {
		t.Fatal(err)
	}
	_, hooks, err = parseSettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if countLeg(hooks["Notification"], "/tmp/dev.toml") != 0 {
		t.Error("dev leg not removed")
	}
	if countLeg(hooks["Notification"], "") != 1 {
		t.Error("production leg removed by mistake")
	}
	if strings.Contains(string(out), "VIGIE_CONFIG=") {
		t.Errorf("dev leg command still present:\n%s", out)
	}
}

func TestInstallUninstallRoundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := Install([]string{"Stop"}, "/bin/claude-fleet", "", 5)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), shellQuote("/bin/claude-fleet")+" report --event=Stop") { // quoted since #513
		t.Errorf("settings missing our hook:\n%s", data)
	}

	if _, err := Uninstall(""); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after uninstall: %v", err)
	}
	if strings.Contains(string(data), "report --event=") {
		t.Errorf("hook not removed:\n%s", data)
	}
}

// TestLegacyFleetConfigLeg checks backward compatibility with legs installed
// before the rename: a FLEET_CONFIG= dev leg is still recognized, and a reinstall
// migrates it to the VIGIE_CONFIG= form instead of leaving a duplicate (#289).
func TestLegacyFleetConfigLeg(t *testing.T) {
	legacy := "FLEET_CONFIG=/tmp/dev.toml /bin/vigie report --event=Stop"
	if !owns(legacy, "/tmp/dev.toml") {
		t.Error("legacy FLEET_CONFIG dev leg not recognized")
	}
	if owns(legacy, "") {
		t.Error("legacy dev leg wrongly matched the production leg")
	}

	base := []byte(`{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"` + legacy + `"}]}]}}`)
	out, err := mergeHooks(base, []string{"Stop"}, "/bin/vigie", "/tmp/dev.toml", 5)
	if err != nil {
		t.Fatal(err)
	}
	_, hooks, err := parseSettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := countLeg(hooks["Stop"], "/tmp/dev.toml"); got != 1 {
		t.Errorf("dev leg count = %d, want 1 (legacy replaced, not duplicated)", got)
	}
	if strings.Contains(string(out), "FLEET_CONFIG=") {
		t.Errorf("legacy leg not migrated to VIGIE_CONFIG:\n%s", out)
	}
	if !strings.Contains(string(out), "VIGIE_CONFIG="+shellQuote("/tmp/dev.toml")) { // quoted since #513
		t.Errorf("reinstalled leg missing VIGIE_CONFIG:\n%s", out)
	}
}

// #644. vigie adds its own hook entry and removes its own; it never needs to read
// inside anyone else's. It decoded every hook into a three-field struct anyway, so
// that it could write the file back — and everything that did not fit that struct
// stopped existing. A conditional hook came back unconditional, a prompt hook lost
// the model it was pinned to, at every watcher start, with no warning.
//
// The fields below are the ones Claude Code documents on a hook. Passing through
// install and then uninstall must return each foreign entry unchanged.
func TestAForeignHookSurvivesInstallAndUninstall(t *testing.T) {
	const foreign = `{
  "permissions": {"allow": ["Bash(ls:*)"]},
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {"type": "command", "command": "prettier --write $F", "timeout": 30, "statusMessage": "formatting", "once": true, "async": true, "if": "Write(*.ts)", "shell": "bash"},
          {"type": "prompt", "prompt": "Is this safe? $ARGUMENTS", "model": "claude-haiku-4-5-20251001", "continueOnBlock": true},
          {"type": "http", "url": "https://hooks.example.com/x", "headers": {"Authorization": "Bearer $T"}, "allowedEnvVars": ["T"]}
        ]
      }
    ],
    "Stop": [
      {"matcher": "", "hooks": [{"type": "agent", "prompt": "Verify tests ran", "timeout": 60}]}
    ]
  }
}`
	installed, err := mergeHooks([]byte(foreign), []string{"Stop", "PostToolUse"}, "/usr/bin/vigie", "/tmp/cfg.toml", 5)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	back, err := removeHooks(installed, "/tmp/cfg.toml")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	var before, after struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(foreign), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(back, &after); err != nil {
		t.Fatal(err)
	}
	for event, entries := range before.Hooks {
		got := after.Hooks[event]
		if len(got) != len(entries) {
			t.Fatalf("%s: %d entries came back, want %d", event, len(got), len(entries))
		}
		for i := range entries {
			// Compared as parsed JSON, not as bytes: the file is reformatted on the
			// way through — indentation and key order — and that is noise, not loss.
			var w, g any
			if err := json.Unmarshal(entries[i], &w); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(got[i], &g); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(w, g) {
				t.Errorf("%s[%d] came back changed:\n want %s\n  got %s", event, i, entries[i], got[i])
			}
		}
	}

	// And the installed file really did carry vigie's own leg, so the round trip
	// above is not passing because nothing was added.
	if !strings.Contains(string(installed), reportMarker) {
		t.Error("install added no vigie hook — the round trip proved nothing")
	}
}

// #644, second shape. An operator may put their own hook in the same entry as
// ours — an entry is keyed by a matcher, and two hooks watching the same tools
// belong together. Removing ours by dropping the whole entry took theirs with it,
// at every watcher start.
func TestAForeignHookSharingOurEntrySurvives(t *testing.T) {
	ours := command("/usr/bin/vigie", "/tmp/cfg.toml", "Stop")
	in := []byte(fmt.Sprintf(`{"hooks":{"Stop":[{"matcher":"Write|Edit","hooks":[
	  {"type":"command","command":"my-precious.sh","statusMessage":"mine","if":"Write(*.ts)"},
	  {"type":"command","command":%q}]}]}}`, ours))

	out, err := mergeHooks(in, []string{"Stop"}, "/usr/bin/vigie", "/tmp/cfg.toml", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"my-precious.sh", "statusMessage", `"if"`, "Write|Edit"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("re-installing dropped %s from the entry it shared with ours:\n%s", want, out)
		}
	}
	if n := strings.Count(string(out), reportMarker); n != 1 {
		t.Errorf("vigie's own hook appears %d times, want 1", n)
	}

	back, err := removeHooks(out, "/tmp/cfg.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), "my-precious.sh") {
		t.Errorf("uninstalling took the foreign hook with it:\n%s", back)
	}
	if strings.Contains(string(back), reportMarker) {
		t.Errorf("uninstalling left one of ours behind:\n%s", back)
	}
}

// Installing must never be fatal (ADR-0009). `"hooks": null` decodes to a nil map
// and writing into one panics, which would kill the watcher at startup rather than
// log and carry on.
func TestSettingsThatAreNotTheExpectedShapeDoNotPanic(t *testing.T) {
	for _, in := range []string{
		`{"hooks":null}`,
		`{"hooks":{"Stop":null}}`,
		`{"hooks":{"Stop":["a string, not an entry"]}}`,
		`{"hooks":{"Stop":[{"matcher":"","hooks":"not an array"}]}}`,
		`{"hooks":{"Stop":[42]}}`,
		``,
	} {
		out, err := mergeHooks([]byte(in), []string{"Stop"}, "/usr/bin/vigie", "/tmp/cfg.toml", 5)
		if err != nil {
			continue // refused is fine; panicking is not
		}
		if _, err := removeHooks(out, "/tmp/cfg.toml"); err != nil {
			t.Errorf("uninstall failed for %s: %v", in, err)
		}
	}
}

// And what it cannot parse, it keeps: an entry that is a string or a number is
// somebody else's by definition, and mangling it would be the bug this all exists
// to fix.
func TestAnUnparsableEntryIsKept(t *testing.T) {
	in := []byte(`{"hooks":{"Stop":["a string, not an entry",42]}}`)
	out, err := mergeHooks(in, []string{"Stop"}, "/usr/bin/vigie", "/tmp/cfg.toml", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a string, not an entry", "42"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("an entry this package cannot read was dropped (%s):\n%s", want, out)
		}
	}
}
