package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The JavaScript clients duplicate rules that also live in Go — deliberately:
// ADR-0011 keeps the operator-dependent half client-side and proves it against
// shared fixtures. What makes that duplication readable is the comments naming
// the Go counterpart, so a reader can open both and compare.
//
// A pointer that leads nowhere does not merely age badly. It stops the comparison
// it exists to enable, and the reader who follows it concludes the note is stale
// rather than that the rule moved. #627 moved code out of `model.go` and split
// `sessionsview.go`; three comments still named the file that no longer exists and
// one still named the old home of `fuzzyMatch` (#671).
//
// Two checks, because a reference can rot in two ways: the file can go, and the
// symbol can move to another file while both files still exist. The second is
// what a plain existence check would have missed.

// crossRefFiles are the shipped JavaScript clients.
var crossRefFiles = []string{
	"../../internal/web/static/lib.js",
	"../../internal/web/static/app.js",
	"../../gnome-extension/lib.js",
	"../../gnome-extension/extension.js",
}

var goPathRe = regexp.MustCompile(`internal/[a-z0-9_/]+\.go`)

// symbolRefRe matches "`name` in internal/pkg/file.go" — the form the comments
// use when they name a specific Go counterpart. The name may be dotted
// (`prefs.visible`); only the last segment is a Go identifier to look for.
var symbolRefRe = regexp.MustCompile("`([A-Za-z0-9_.]+)` in (internal/[a-z0-9_/]+\\.go)")

func readClient(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // paths are our own committed sources
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b), true
}

func TestEveryGoFileTheClientsCiteExists(t *testing.T) {
	for _, path := range crossRefFiles {
		src, ok := readClient(t, path)
		if !ok {
			continue
		}
		for _, ref := range goPathRe.FindAllString(src, -1) {
			if _, err := os.Stat(filepath.Join("../..", ref)); err != nil {
				t.Errorf("%s cites %s, which does not exist — the comment sends a reader nowhere",
					filepath.Base(path), ref)
			}
		}
	}
}

func TestEverySymbolTheClientsCiteIsInTheFileTheyName(t *testing.T) {
	for _, path := range crossRefFiles {
		src, ok := readClient(t, path)
		if !ok {
			continue
		}
		// Comments wrap, so a reference can straddle two `// ` lines. Fold the
		// continuations before matching, or half of them are invisible here.
		flat := strings.ReplaceAll(src, "\n//", "")
		for _, m := range symbolRefRe.FindAllStringSubmatch(flat, -1) {
			name, goFile := m[1], m[2]
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:] // `prefs.visible` → visible
			}
			body, err := os.ReadFile(filepath.Join("../..", goFile)) //nolint:gosec // paths are our own sources
			if err != nil {
				continue // the existence check above owns that failure
			}
			if !strings.Contains(string(body), name) {
				t.Errorf("%s says `%s` is in %s; it is not — the rule moved and the comment did not",
					filepath.Base(path), name, goFile)
			}
		}
	}
}
