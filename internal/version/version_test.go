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

// TestMatch covers the fleet's single version rule (#384): release builds compare
// by version string, and any dev build compares by commit — so a "dev" == "dev"
// match across two different commits stays a miss.
func TestMatch(t *testing.T) {
	cases := []struct {
		name          string
		aVer, aCommit string
		bVer, bCommit string
		want          bool
	}{
		{"same release", "0.4.1", "abc", "0.4.1", "abc", true},
		{"same release, different commit", "0.4.1", "abc", "0.4.1", "def", true},
		{"release drift", "0.4.1", "abc", "0.5.0", "abc", false},
		{"dev, same commit", "dev", "abc", "dev", "abc", true},
		{"dev, different commit", "dev", "abc", "dev", "def", false},
		{"dev vs release, same commit", "dev", "abc", "0.5.0", "abc", true},
		{"dev vs release, different commit", "dev", "abc", "0.5.0", "def", false},
		{"undeclared vs release", "", "", "0.5.0", "abc", false},
	}
	for _, c := range cases {
		if got := Match(c.aVer, c.aCommit, c.bVer, c.bCommit); got != c.want {
			t.Errorf("%s: Match(%q,%q,%q,%q) = %v, want %v",
				c.name, c.aVer, c.aCommit, c.bVer, c.bCommit, got, c.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	for _, c := range []struct{ v, commit, want string }{
		{"0.4.1", "abc1234", "0.4.1 (commit abc1234)"},
		{"0.4.1", "", "0.4.1"},
		{"dev", "none", "dev"},
		{"", "", "an undeclared build"},
	} {
		if got := Describe(c.v, c.commit); got != c.want {
			t.Errorf("Describe(%q, %q) = %q, want %q", c.v, c.commit, got, c.want)
		}
	}
}
