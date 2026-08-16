package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// #513. The hook command written into settings.json was built by concatenation:
//
//	fmt.Sprintf("%s report --event=%s", binPath, event)
//
// `binPath` comes from os.Executable() and the config path from the config
// resolution; either can contain a space. Claude Code runs the string through a
// shell, which then executes `/home/me/My` with `Tools/vigie` as an argument.
//
// It fails the way this project keeps finding worst: the hook is installed,
// settings.json looks right, `vigie hooks install` reports success, and no event
// ever arrives — so `waiting`, the one status only a hook can see, is permanently
// invisible with nothing saying why.

// runCommand executes a built hook command through a shell, the way Claude Code
// does, and returns what the "binary" received. Asserting on the string alone
// would not have caught this: the string contains the path either way.
func runCommand(t *testing.T, cmd string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput() //nolint:gosec // a command this package built
	if err != nil {
		t.Fatalf("running %q: %v\n%s", cmd, err, out)
	}
	return strings.TrimSpace(string(out))
}

// fakeBinary writes an executable that prints its arguments, at a path of the
// caller's choosing — including one with a space in it.
func fakeBinary(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho \"$@\"\n"), 0o700); err != nil { //nolint:gosec // test binary must be executable
		t.Fatal(err)
	}
	return path
}

func TestAHookCommandSurvivesASpaceInTheBinaryPath(t *testing.T) {
	bin := fakeBinary(t, filepath.Join(t.TempDir(), "My Tools"), "vigie")

	got := runCommand(t, command(bin, "", "Stop"))
	if got != "report --event=Stop" {
		t.Errorf("the binary received %q, want %q — the shell split the path", got, "report --event=Stop")
	}
}

func TestAHookCommandSurvivesASpaceInTheConfigPath(t *testing.T) {
	dir := t.TempDir()
	bin := fakeBinary(t, dir, "vigie")
	cfg := filepath.Join(dir, "My Configs", "dev.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("server_url = \"http://x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The binary prints its args; the env assignment must not become one of them,
	// nor break the command in two.
	got := runCommand(t, command(bin, cfg, "Stop"))
	if got != "report --event=Stop" {
		t.Errorf("the binary received %q — the config path split the command", got)
	}
}

// A quoted leg must still be recognized as ours, or a reinstall adds a second
// copy of every hook instead of replacing the first.
func TestAQuotedLegIsStillOurs(t *testing.T) {
	cfg := "/tmp/My Configs/dev.toml"
	cmd := command("/opt/My Tools/vigie", cfg, "Stop")

	if !owns(cmd, cfg) {
		t.Errorf("a leg this package just built is not recognized as its own:\n  %s", cmd)
	}
	if owns(cmd, "") {
		t.Error("a dev leg was mistaken for the production leg")
	}
	prod := command("/opt/My Tools/vigie", "", "Stop")
	if !owns(prod, "") {
		t.Errorf("the production leg is not recognized as its own:\n  %s", prod)
	}
	if owns(prod, cfg) {
		t.Error("the production leg was mistaken for a dev leg")
	}
}

// Legs installed before this change are unquoted and still in operators'
// settings.json. They must keep matching, or the first reinstall duplicates them.
func TestAnUnquotedLegFromBeforeIsStillRecognized(t *testing.T) {
	legacy := "VIGIE_CONFIG=/tmp/dev.toml /usr/bin/vigie report --event=Stop"
	if !owns(legacy, "/tmp/dev.toml") {
		t.Error("a leg installed before quoting is no longer recognized — a reinstall would duplicate it")
	}
	legacyProd := "/usr/bin/vigie report --event=Stop"
	if !owns(legacyProd, "") {
		t.Error("an unquoted production leg is no longer recognized")
	}
}
