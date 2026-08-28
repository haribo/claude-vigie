package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// #589 replaced the operator's own machine names, account names and home paths
// with placeholders, and closed as done. It swept the Go tests and the design
// docs and did not reach the JavaScript suites, which were not in its list —
// twelve occurrences sat there for another two weeks (#631).
//
// It survived because the sweep was a list of files rather than a check. This is
// the check: the repository may name itself, and it may not name the machine it
// happens to be developed on. Nothing here fails if a *new* fixture uses a
// placeholder; it fails when a real name arrives.
//
// Adding to `realNames` is how a future leak is closed. Removing an entry needs a
// reason, because each one is a name that reached a commit once.
var realNames = regexp.MustCompile(`(?i)\b(minet|nico)\b`)

func TestNoRealNameIsCommitted(t *testing.T) {
	roots := []string{"../../internal", "../../test", "../../docs", "../../cmd",
		"../../gnome-extension", "../../site", "../../tools"}
	checked := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			switch filepath.Ext(path) {
			case ".go", ".js", ".mjs", ".json", ".md", ".toml", ".html", ".css":
			default:
				return nil
			}
			// This file holds the pattern, so it necessarily contains the names.
			if strings.HasSuffix(path, "placeholders_test.go") {
				return nil
			}
			checked++
			for i, line := range strings.Split(read(t, path), "\n") {
				// `unicode` and friends contain one of the names as a substring; the
				// word boundaries in the pattern already exclude them, and this keeps
				// the failure message honest about what it matched.
				if m := realNames.FindString(line); m != "" {
					t.Errorf("%s:%d names %q — the repository may name itself, not the machine "+
						"it is developed on (CLAUDE.md, #589, #631)", path, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if checked < 100 {
		t.Fatalf("only %d files walked — the extraction is broken, not the tree", checked)
	}
}
