package tui

import (
	"encoding/json"
	"os"
	"testing"
)

// Several rules in this package live twice — once here and once in the
// dashboard's JavaScript — because they are functions of what the operator typed
// or chose, which ADR-0011 deliberately leaves client-side. Each is proved against
// a case list under test/fixtures/ that both suites read.
//
// Reading one is always the same three steps and one interesting line. The
// interesting line is the emptiness check, and it stays with each test because
// what it must say differs: a fixture whose section is missing means the
// extraction is broken, not the code, and the message has to name the section.
func loadFixture[T any](t *testing.T, name string) T {
	t.Helper()
	path := "../../test/fixtures/" + name
	b, err := os.ReadFile(path) //nolint:gosec // a literal path under our own test tree
	if err != nil {
		t.Fatalf("reading the shared fixture %s: %v", name, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("parsing the shared fixture %s: %v", name, err)
	}
	return out
}
