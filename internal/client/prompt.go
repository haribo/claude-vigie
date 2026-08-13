package client

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/haribo/claude-vigie/internal/config"
)

// `vigie init` needs two values it cannot invent: the server URL and the shared
// token. Passing them as flags writes the token into the shell history of every
// machine, permanently — the same reason it has no place in a systemd unit. So
// they are resolved in order of decreasing ceremony, and only asked for when
// there is somebody to answer (#407).

// stdinIsTerminal is a seam: tests drive the non-interactive branches without a
// pty, and the interactive one without a human.
var stdinIsTerminal = func() bool { return term.IsTerminal(os.Stdin.Fd()) }

// promptFn reads one answer. Indirected for the same reason.
var promptFn = ask

// The prompt labels are constants so the tests assert against the same strings
// the operator sees. The token is echoed as asterisks (see secret.go), which is
// why the label needs no "input hidden" hint.
const (
	labelServer = "Server URL (e.g. http://localhost:8080)"
	//nolint:gosec // G101 fires on the identifier: this is a prompt label, not a credential
	labelToken = "Token"
)

// errNoEndpoint is returned when nothing supplied the values and nothing can ask.
var errNoEndpoint = errors.New("no server URL or token: pass --server and --token, " +
	"set VIGIE_SERVER and VIGIE_TOKEN, or run `vigie init` from a terminal")

// resolveEndpoint fills the server URL and token from, in order: the flags, the
// environment, then an interactive prompt. A non-interactive caller that supplied
// neither gets errNoEndpoint rather than a prompt nobody can answer — which would
// hang a container or a provisioning run forever.
func resolveEndpoint(server, token string) (string, string, error) {
	if server == "" {
		server = config.EnvServer()
	}
	if token == "" {
		token = config.EnvToken()
	}
	if server != "" && token != "" {
		return server, token, nil
	}
	if !stdinIsTerminal() {
		return "", "", errNoEndpoint
	}

	var err error
	if server == "" {
		if server, err = promptFn(labelServer, false); err != nil {
			return "", "", err
		}
	}
	if token == "" {
		// Read without echo: a prompt that displays the token merely moves the
		// secret from the shell history to the terminal scrollback.
		if token, err = promptFn(labelToken, true); err != nil {
			return "", "", err
		}
	}
	if server == "" || token == "" {
		return "", "", errors.New("server URL and token are both required")
	}
	return server, token, nil
}

// ask writes the label to stderr — stdout stays clean for anything piping this
// command — and reads one line, without echo when the answer is a secret.
func ask(label string, secret bool) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	if secret {
		v, err := readMasked(os.Stdin, os.Stderr)
		if err != nil {
			return "", fmt.Errorf("reading the token: %w", err)
		}
		return strings.TrimSpace(v), nil
	}
	line, err := readLine(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	return strings.TrimSpace(line), nil
}

// readLine reads one line **unbuffered**. A bufio.Reader reads ahead by up to its
// buffer size and would swallow the following line — here the token that
// term.ReadPassword is about to ask for, leaving it waiting forever for input
// already consumed. A human typing one line at a time never triggers it; a pipe
// or a fast paste always does. A prompt reads a handful of bytes, so
// byte-at-a-time costs nothing and cannot over-read.
func readLine(f *os.File) (string, error) {
	var out []byte
	buf := make([]byte, 1)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return string(out), nil
			}
			out = append(out, buf[0])
		}
		if err != nil {
			if len(out) > 0 {
				return string(out), nil // a final line with no trailing newline
			}
			return "", err
		}
	}
}
