package version

import (
	"strings"
	"testing"
)

func TestStringUsesName(t *testing.T) {
	for _, name := range []string{"claude-fleet", "claude-fleetd"} {
		out := String(name)
		if !strings.HasPrefix(out, name+" ") {
			t.Errorf("String(%q) = %q, want it to start with the binary name", name, out)
		}
	}
}
