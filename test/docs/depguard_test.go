package docs_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// #556. `depguard`'s `client-minimal` rule is the machine-checked half of
// ADR-0003: the client binary must never link the server, the store, the web
// dashboard or Prometheus. Which files it applies to is a hand-kept list in
// `.golangci.yml`, and a hand-kept list of what to check is not a check.
//
// It has already been wrong twice. It named three packages until #515 widened it
// to eight, with the comment "Every client-side package, not the three that
// happened to be listed" — and it was still seven short: `api`, `apiclient`,
// `compaction`, `config`, `status`, `transcript` and `version` all ship in
// `vigie` and sat outside the barrier, so an import of `internal/store` added to
// any of them would have compiled, linked and passed CI.
//
// This is the same guard `routes_test.go` gives the API table: derive the truth
// from the thing itself — here the dependency graph of `cmd/vigie` — instead of
// comparing two lists a human keeps.
//
// It lives beside the other declaration-versus-reality guards rather than in a
// directory of its own; the linter config is a declaration like the docs are.

// clientPackages are the repository's own packages that the client binary links.
func clientPackages(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "../../cmd/vigie").Output()
	if err != nil {
		t.Fatalf("listing the client's dependencies: %v", err)
	}
	const prefix = "github.com/haribo/claude-vigie/internal/"
	var pkgs []string
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok && p != "" {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		t.Fatal("no internal package found in the client's dependencies — the extraction is broken, not the config")
	}
	return pkgs
}

// guardedPackages are the packages `client-minimal`'s `files:` globs cover.
func guardedPackages(t *testing.T) map[string]bool {
	t.Helper()
	body := read(t, "../../.golangci.yml")
	i := strings.Index(body, "client-minimal:")
	if i < 0 {
		t.Fatal(".golangci.yml has no `client-minimal:` rule — this guard needs updating")
	}
	rest := body[i:]
	j := strings.Index(rest, "files:")
	k := strings.Index(rest, "deny:")
	if j < 0 || k < 0 || k < j {
		t.Fatal("`client-minimal` has no `files:` … `deny:` block — this guard needs updating")
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`internal/([a-z]+)`).FindAllStringSubmatch(rest[j:k], -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("no package parsed out of the `files:` block")
	}
	return out
}

func TestDepguardCoversEveryClientPackage(t *testing.T) {
	guarded := guardedPackages(t)
	var missing []string
	for _, p := range clientPackages(t) {
		if !guarded[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Errorf("these packages ship in the vigie binary and are outside the depguard barrier: %v\n"+
			"add them to `client-minimal.files` in .golangci.yml (ADR-0003)", missing)
	}
}
