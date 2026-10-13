package usage

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// On macOS Claude Code keeps its credentials in the login Keychain, not in
// ~/.claude/.credentials.json (ADR-0018, #869). These tests replace the `security` run:
// the real one reads the operator's own token, which no test may touch.

// keychain is a fake login Keychain: service name → item, or → the error
// `security` fails with. A service it does not list is not found (exit 44).
type keychain map[string]any

// fakeSecurity substitutes the Keychain read for one test and records the
// services asked for.
func fakeSecurity(t *testing.T, kc keychain) *[]string {
	t.Helper()
	var asked []string
	notFound := exitErr(t, "44")
	orig := runSecurity
	runSecurity = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) != 4 || args[0] != "find-generic-password" || args[1] != "-s" || args[3] != "-w" {
			t.Fatalf("security argv = %q", args)
		}
		asked = append(asked, args[2])
		switch v := kc[args[2]].(type) {
		case string:
			return []byte(v + "\n"), nil
		case error:
			return nil, v
		default:
			return nil, notFound
		}
	}
	t.Cleanup(func() { runSecurity = orig })
	return &asked
}

// item is a Keychain item's payload, in the shape Claude Code writes.
func item(token string, expiresAt int64) string {
	return `{"claudeAiOauth":{"accessToken":"` + token + `","expiresAt":` + strconv.FormatInt(expiresAt, 10) + `}}`
}

// exitErr returns a real *exec.ExitError carrying code, as `security` produces.
func exitErr(t *testing.T, code string) error {
	t.Helper()
	err := exec.Command("/bin/sh", "-c", "exit "+code).Run() //nolint:gosec // fixed shell, numeric test code
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("no exit error for code %s: %v", code, err)
	}
	return err
}

// Claude Code names the item after its config directory: the first 8 hex digits
// of sha256 of the path, as observed (ADR-0018). The reference value was computed
// outside Go: printf '%s' /Users/you/.claude | shasum -a 256.
func TestKeychainServicesNameTheConfigDirectory(t *testing.T) {
	got := keychainServices("/Users/you")
	want := []string{"Claude Code-credentials-34f6278e", "Claude Code-credentials"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("keychainServices = %q, want %q", got, want)
	}
}

func TestReadTokenFromTheKeychain(t *testing.T) {
	t.Setenv("HOME", "/Users/you") // and no file there: the Keychain must answer
	fakeSecurity(t, keychain{"Claude Code-credentials-34f6278e": item("kc-tok", 2000)})

	if got, err := readToken(); err != nil || got != "kc-tok" {
		t.Fatalf("readToken = (%q, %v), want kc-tok", got, err)
	}
}

// Both items can exist — the observed case: an unsuffixed one left from before
// the config directory was named, expired months ago, beside the live one. The
// one that expires later is the one Claude Code is still refreshing, whichever
// name it carries.
func TestReadTokenPrefersTheItemThatExpiresLater(t *testing.T) {
	t.Setenv("HOME", "/Users/you")
	for name, kc := range map[string]keychain{
		"suffixed is live": {
			"Claude Code-credentials-34f6278e": item("live", 2000),
			"Claude Code-credentials":          item("stale", 1000),
		},
		"unsuffixed is live": {
			"Claude Code-credentials-34f6278e": item("stale", 1000),
			"Claude Code-credentials":          item("live", 2000),
		},
	} {
		t.Run(name, func(t *testing.T) {
			fakeSecurity(t, kc)
			if got, err := readToken(); err != nil || got != "live" {
				t.Errorf("readToken = (%q, %v), want live", got, err)
			}
		})
	}
}

// No item at all is a Mac where Claude Code fell back to the file.
func TestReadTokenFallsBackToTheFileWhenTheKeychainHasNoItem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCreds(t, "file-tok")
	fakeSecurity(t, keychain{})

	if got, err := readToken(); err != nil || got != "file-tok" {
		t.Fatalf("readToken = (%q, %v), want file-tok", got, err)
	}
}

// A refusal — a denied access dialog, a locked Keychain — is reported as itself
// when nothing else answers. Falling back to a file that is not there would
// replace it with "no such file", which is the message that hid this whole gap.
func TestReadTokenReportsAKeychainRefusal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fakeSecurity(t, keychain{"Claude Code-credentials": exitErr(t, "51")})

	_, err := readToken()
	if err == nil || !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("readToken error = %v, want a keychain error", err)
	}
}

// One item refusing does not hide another that answers.
func TestReadTokenUsesTheItemThatAnswers(t *testing.T) {
	t.Setenv("HOME", "/Users/you")
	fakeSecurity(t, keychain{
		"Claude Code-credentials-34f6278e": exitErr(t, "51"),
		"Claude Code-credentials":          item("ok", 1000),
	})
	if got, err := readToken(); err != nil || got != "ok" {
		t.Errorf("readToken = (%q, %v), want ok", got, err)
	}
}

// The guard for every other test in this module: the real reader refuses to run
// inside a test binary, so a test that forgets the fake reads nothing rather
// than the operator's token.
func TestTheRealKeychainReaderRefusesInTests(t *testing.T) {
	if _, err := securityCommand(context.Background(), "find-generic-password", "-s", "vigie-never-read", "-w"); !errors.Is(err, errKeychainInTest) {
		t.Errorf("real reader under test = %v, want errKeychainInTest", err)
	}
}
