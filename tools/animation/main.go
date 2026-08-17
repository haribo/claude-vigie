// Command animation writes the README's "A session can call you" asset.
//
// Run it from the repository root after editing internal/animation/template.svg
// or its palettes:
//
//	just docs-animation
//
// The committed files are compared against a fresh render by the package's test,
// so forgetting to run this fails the build rather than shipping a stale asset
// (#450).
package main

import (
	"fmt"
	"os"

	"github.com/haribo/claude-vigie/internal/animation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "animation: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	for _, p := range animation.Themes() {
		svg, err := animation.Render(p)
		if err != nil {
			return err
		}
		for _, path := range animation.Targets(p) {
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil { //nolint:gosec // a committed asset, world-readable on purpose
				return fmt.Errorf("writing %s: %w", path, err)
			}
			fmt.Println("wrote", path)
		}
	}
	return nil
}
