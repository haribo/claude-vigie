package client

import "testing"

// TestRunDispatch covers the top-level command dispatch — the exit codes for the
// no-network commands (the side-effecting subcommands have their own tests).
func TestRunDispatch(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 2},
		{"unknown command", []string{"nope"}, 2},
		{"help", []string{"help"}, 0},
		{"--help", []string{"--help"}, 0},
		{"version", []string{"version"}, 0},
		{"-v", []string{"-v"}, 0},
	}
	for _, c := range cases {
		if got := Run(c.args); got != c.want {
			t.Errorf("%s: Run(%v) = %d, want %d", c.name, c.args, got, c.want)
		}
	}
}
