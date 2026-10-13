package usage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// On macOS Claude Code keeps its credentials in the login Keychain (ADR-0018).
// They are read through /usr/bin/security, which the item's access list already
// trusts — Claude Code stores it that way — so the read raises no dialog and
// widens nothing. A native Keychain call was rejected: it needs cgo, and an
// unsigned binary's access grant is lost at every rebuild.

const (
	securityPath    = "/usr/bin/security" // absolute: a PATH lookup could be shadowed
	keychainService = "Claude Code-credentials"
	keychainTimeout = 5 * time.Second
	// errSecItemNotFound, as `security` reports it.
	exitItemNotFound = 44
)

// errKeychainInTest is what the real reader answers inside a test binary.
var errKeychainInTest = errors.New("the keychain is never read from a test binary")

// runSecurity is a seam: tests answer for `security`.
var runSecurity = securityCommand

// securityCommand runs /usr/bin/security and returns its stdout. Inside a test
// binary it refuses: the Keychain ignores HOME, so a test isolated the way every
// other test here is would otherwise read the operator's real token.
func securityCommand(ctx context.Context, args ...string) ([]byte, error) {
	if testing.Testing() {
		return nil, errKeychainInTest
	}
	ctx, cancel := context.WithTimeout(ctx, keychainTimeout)
	defer cancel()
	// stderr is dropped, never wrapped into an error: it is the one channel that
	// could carry anything of the item back into a log.
	return exec.CommandContext(ctx, securityPath, args...).Output() //nolint:gosec // fixed binary, fixed arguments
}

// keychainServices lists the item names Claude Code may have stored its
// credentials under, for the config directory vigie watches (~/.claude).
// Claude Code suffixes the name with the first 8 hex digits of sha256 of its
// config directory — observed with CLAUDE_CONFIG_DIR set — and an item without
// the suffix can remain beside it from before (ADR-0018). The directory is
// derived from HOME rather than read from CLAUDE_CONFIG_DIR: a LaunchAgent does
// not carry the variable, and ~/.claude is the directory vigie reads anyway.
func keychainServices(home string) []string {
	sum := sha256.Sum256([]byte(filepath.Join(home, ".claude")))
	return []string{keychainService + "-" + hex.EncodeToString(sum[:])[:8], keychainService}
}

// readCredentials returns Claude Code's stored credentials: the Keychain item
// that expires last, or the file when the Keychain holds none — the fallback
// Claude Code itself uses where the Keychain is unavailable. The latest expiry
// decides rather than the name, because which name is live depends on how
// Claude Code was started, and the stale one stays behind.
func readCredentials() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home dir: %w", err)
	}
	var best []byte
	bestExpiry := int64(-1)
	var refusal error
	for _, svc := range keychainServices(home) {
		out, err := runSecurity(context.Background(), "find-generic-password", "-s", svc, "-w")
		var ee *exec.ExitError
		switch {
		case errors.Is(err, errKeychainInTest), errors.As(err, &ee) && ee.ExitCode() == exitItemNotFound:
			continue
		case err != nil:
			if refusal == nil {
				refusal = err
			}
			continue
		}
		out = bytes.TrimSpace(out)
		if exp := expiresAt(out); exp > bestExpiry {
			best, bestExpiry = out, exp
		}
	}
	switch {
	case best != nil:
		return best, nil
	case refusal != nil:
		return nil, fmt.Errorf("reading credentials from the keychain: %w", refusal)
	default:
		return readCredentialsFile()
	}
}

// expiresAt reads the access token's expiry (milliseconds since the epoch) from
// an item; 0 when it carries none, so an item with an expiry always wins.
func expiresAt(item []byte) int64 {
	var c struct {
		ClaudeAiOauth struct {
			ExpiresAt int64 `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(item, &c) != nil {
		return 0
	}
	return c.ClaudeAiOauth.ExpiresAt
}
