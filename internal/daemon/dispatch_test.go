package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `Run` is the CLI contract of vigied: which subcommand runs, and what an
// operator or a script gets back when they mistype one. It was uncovered (#442),
// as was `runToken` — `token_test.go` exercises `resolveToken`, never the command.

func TestRunRejectsWhatItCannotDispatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"no arguments", nil, 2},
		{"unknown command", []string{"srve"}, 2},
		{"a flag in place of a command", []string{"--db=x"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Run(tc.args); got != tc.want {
				t.Errorf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunHandlesHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{
		{"help"}, {"--help"}, {"-h"},
		{"version"}, {"--version"}, {"-v"},
	} {
		if got := Run(args); got != 0 {
			t.Errorf("Run(%v) = %d, want 0", args, got)
		}
	}
}

// TestUsageListsEverySubcommand: the help text is the only place an operator
// discovers what the binary can do, and it silently fell behind when
// `stats-repair` was added (#436).
func TestUsageListsEverySubcommand(t *testing.T) {
	var b strings.Builder
	usage(&b)
	out := b.String()

	for _, cmd := range []string{"serve", "token", "stats-repair", "version", "help"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("usage does not mention %q", cmd)
		}
	}
}

// TestRunTokenPrintsAToken drives the command itself, not just resolveToken: a
// fresh database has no token, so the command must generate, persist and print
// one — and printing the same value on a second call is what makes it usable for
// connecting a client.
func TestRunTokenPrintsAToken(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")

	first := captureStdout(t, func() int { return runToken([]string{"-db", db}) })
	if strings.TrimSpace(first) == "" {
		t.Fatal("no token printed")
	}

	second := captureStdout(t, func() int { return runToken([]string{"-db", db}) })
	if first != second {
		t.Errorf("token changed between calls: %q then %q", strings.TrimSpace(first), strings.TrimSpace(second))
	}
}

func TestRunTokenFailsOnAnUnusableDatabase(t *testing.T) {
	// A directory where the database file should be: opening it must fail rather
	// than panic or print something an operator would mistake for a token.
	dir := filepath.Join(t.TempDir(), "not-a-file")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if got := runToken([]string{"-db", dir}); got == 0 {
		t.Error("exit = 0 on an unusable database")
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func() int) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	code := fn()
	os.Stdout = orig
	_ = w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, rerr := r.Read(buf)
		b.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	_ = r.Close()

	if code != 0 {
		t.Fatalf("command exited %d: %s", code, b.String())
	}
	return b.String()
}
