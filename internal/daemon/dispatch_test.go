package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
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

// newTokenDB creates an initialized, empty database and returns its path — the
// state a daemon leaves behind when it was given its token through the
// environment and had none of its own to persist.
func newTokenDB(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "t.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestRunTokenPrintsTheStoredToken keeps what made the command useful: the same
// value on every call, so it can be handed to a client.
//
// This test used to require the opposite of what it requires now. It read "a
// fresh database has no token, so the command must generate, persist and print
// one", which is #657: run from an operator shell against a daemon holding its
// token in the environment, the command found an empty store and invented a
// secret no server had ever heard of.
func TestRunTokenPrintsTheStoredToken(t *testing.T) {
	t.Setenv(tokenEnv, "")
	db := newTokenDB(t)
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMeta(context.Background(), "token", "stored-tok"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	first := captureStdout(t, func() int { return runToken([]string{"-db", db}) })
	if strings.TrimSpace(first) != "stored-tok" {
		t.Fatalf("printed %q, want the stored token", strings.TrimSpace(first))
	}
	second := captureStdout(t, func() int { return runToken([]string{"-db", db}) })
	if first != second {
		t.Errorf("token changed between calls: %q then %q", strings.TrimSpace(first), strings.TrimSpace(second))
	}
}

// The #657 regression. A command that answers a question must not write one.
func TestRunTokenDoesNotMintWhenThereIsNothingToPrint(t *testing.T) {
	t.Setenv(tokenEnv, "")
	db := newTokenDB(t)

	out, code := captureRun(t, func() int { return runToken([]string{"-db", db}) })

	if code == 0 {
		t.Error("exit = 0 with no token to print; a script cannot tell the answer apart from a token")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("printed %q on stdout; an operator would hand that to a machine", strings.TrimSpace(out))
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if v, ok, _ := st.GetMeta(context.Background(), "token"); ok {
		t.Errorf("the command wrote token=%q into the database it was only asked to read", v)
	}
}

// When this process does carry the variable, that is the token in use and the
// command can answer for it.
func TestRunTokenPrintsTheEnvironmentToken(t *testing.T) {
	t.Setenv(tokenEnv, "env-tok")
	db := newTokenDB(t)

	out := captureStdout(t, func() int { return runToken([]string{"-db", db}) })
	if strings.TrimSpace(out) != "env-tok" {
		t.Errorf("printed %q, want the environment token", strings.TrimSpace(out))
	}
}

// A path that is not there is not a database with no token: reporting it as one
// would send the operator looking for a missing secret rather than a typo, and
// opening it would create the file the message then describes.
func TestRunTokenDoesNotCreateAMissingDatabase(t *testing.T) {
	t.Setenv(tokenEnv, "")
	db := filepath.Join(t.TempDir(), "absent.db")

	if code := runToken([]string{"-db", db}); code == 0 {
		t.Error("exit = 0 for a database that does not exist")
	}
	if _, err := os.Stat(db); err == nil {
		t.Error("the command created the database it was only asked to read")
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

// captureStdout runs fn with os.Stdout redirected and returns what it printed,
// failing the test if fn did not succeed.
func captureStdout(t *testing.T, fn func() int) string {
	t.Helper()
	out, code := captureRun(t, fn)
	if code != 0 {
		t.Fatalf("command exited %d: %s", code, out)
	}
	return out
}

// captureRun is the same capture without the verdict, for the cases where a
// non-zero exit is the thing being asserted.
func captureRun(t *testing.T, fn func() int) (string, int) {
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

	return b.String(), code
}
