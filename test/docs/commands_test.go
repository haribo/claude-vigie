package docs_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// #510. `vigie init` stopped installing hooks (#415, ADR-0009) and three places
// kept saying it did — including the README's command table, which is what an
// operator copies when setting a machine up. The README even contradicted itself
// fifty lines apart, and `vigie init -h` was right while `vigie help` was wrong.
//
// The failure is the one this repository keeps meeting: a document checked for
// its shape and never against the thing it describes. `docs_test.go` verifies
// titles, status lines, package names and the old brand; nothing compared the
// command table to the CLI.
//
// WHAT THIS DOES NOT COVER: the *descriptions*. Prose cannot be compared without
// writing it twice, and a second copy drifts as readily as the first. What is
// compared is the set of commands — the part an operator acts on, and the part
// that silently gains or loses an entry.

// readmeCommands are the commands the README's "Two binaries" block lists for
// one binary. It reads both: the block is titled "Two binaries" and the guard
// covered one of them, so `vigied stats-repair` was documented in
// architecture.md, printed by the CLI, and absent from the README with nothing
// to notice it (#556).
func readmeCommands(t *testing.T, binary string) map[string]bool {
	t.Helper()
	body := read(t, "../../README.md")
	block := regexp.MustCompile("(?s)## Two binaries.*?```\n(.*?)```").FindStringSubmatch(body)
	if block == nil {
		t.Fatal("the README has no `## Two binaries` block — this guard needs updating")
	}
	out := map[string]bool{}
	for _, line := range strings.Split(block[1], "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == binary {
			out[fields[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("no `%s <command>` line found in the block — the extraction is broken", binary)
	}
	return out
}

// helpCommands are the commands `<binary> help` prints.
func helpCommands(t *testing.T, binary string) map[string]bool {
	t.Helper()
	// The binary comes from a literal list in the caller, not from anything a
	// user or a file supplies — same reasoning as internal/tui/attention.go.
	//nolint:gosec // fixed command, argument from a hard-coded set of two
	out, err := exec.Command("go", "run", "../../cmd/"+binary, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("running %s help: %v\n%s", binary, err, out)
	}
	body := string(out)
	i := strings.Index(body, "Commands:")
	if i < 0 {
		t.Fatalf("`vigie help` prints no Commands section:\n%s", body)
	}
	cmds := map[string]bool{}
	for _, line := range strings.Split(body[i:], "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(line, "  ") {
			continue
		}
		cmds[fields[0]] = true
	}
	if len(cmds) == 0 {
		t.Fatalf("no command parsed out of:\n%s", body)
	}
	return cmds
}

// The README's table and the CLI list the same commands, for *both* binaries.
// `help` and `version` are the two the README leaves out on purpose — they are
// not part of setting a machine up.
func TestTheReadmeTableMatchesTheCli(t *testing.T) {
	notInReadme := map[string]bool{"help": true, "version": true}
	for _, binary := range []string{"vigie", "vigied"} {
		readme, help := readmeCommands(t, binary), helpCommands(t, binary)
		for cmd := range help {
			if !readme[cmd] && !notInReadme[cmd] {
				t.Errorf("`%s %s` exists and the README's command table does not list it", binary, cmd)
			}
		}
		for cmd := range readme {
			if !help[cmd] {
				t.Errorf("the README's command table lists `%s %s`, which the CLI does not have", binary, cmd)
			}
		}
	}
}

// The specific claim that was false, asserted in both places at once: `init`
// writes the config, and does not install hooks (#415, ADR-0009).
func TestNothingClaimsThatInitInstallsHooks(t *testing.T) {
	out, err := exec.Command("go", "run", "../../cmd/vigie", "help").CombinedOutput()
	if err != nil {
		t.Fatalf("running vigie help: %v\n%s", err, out)
	}
	claim := regexp.MustCompile(`(?i)init\s.*install\w*\s+hooks`)
	if m := claim.FindString(string(out)); m != "" {
		t.Errorf("`vigie help` still says init installs hooks: %q", m)
	}
	for _, p := range []string{"../../README.md", "../../docs/architecture.md"} {
		for _, line := range strings.Split(read(t, p), "\n") {
			if strings.Contains(line, "init") && claim.MatchString(line) {
				t.Errorf("%s still says init installs hooks: %q", p, strings.TrimSpace(line))
			}
		}
	}
}
