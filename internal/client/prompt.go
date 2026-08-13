package client

import (
	"bufio"
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
		if server, err = promptFn("Server URL (e.g. http://localhost:8080)", false); err != nil {
			return "", "", err
		}
	}
	if token == "" {
		// Read without echo: a prompt that displays the token merely moves the
		// secret from the shell history to the terminal scrollback.
		if token, err = promptFn("Token", true); err != nil {
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
		b, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Fprintln(os.Stderr) // ReadPassword swallows the newline the user typed
		if err != nil {
			return "", fmt.Errorf("reading the token: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	return strings.TrimSpace(line), nil
}
