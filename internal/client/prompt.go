package client

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// `vigie init` needs three values it cannot invent: the server URL, the shared
// token, and the name this machine shows under. It asks for all three.
//
// There are no flags and no environment variables for them. Passing a token as a
// flag writes it into the shell history of every machine, permanently; and the
// non-interactive path they existed for had no user — nothing in this repository
// ever installed vigie unattended (#415). Adding an input back the day one appears
// costs nothing and breaks nobody; carrying it meanwhile costs documentation,
// tests, and a command that looks harder than the two questions it asks.

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
	labelToken   = "Token"
	labelMachine = "Machine name"
)

// errNoTerminal is returned when there is nobody to answer the questions.
var errNoTerminal = errors.New("`vigie init` asks for the server URL and token, " +
	"so it needs a terminal — run it from one")

// setup is what init asks for and writes.
type setup struct {
	server  string
	token   string
	machine string
}

// askSetup asks the three questions. Without a terminal it fails rather than
// blocking on a prompt nobody can answer, which would hang a scripted run forever.
func askSetup(defaultMachine string) (setup, error) {
	if !stdinIsTerminal() {
		return setup{}, errNoTerminal
	}
	server, err := promptFn(labelServer, false)
	if err != nil {
		return setup{}, err
	}
	// Read without echo: a prompt that displays the token merely moves the secret
	// from the shell history to the terminal scrollback.
	token, err := promptFn(labelToken, true)
	if err != nil {
		return setup{}, err
	}
	// The hostname is right most of the time and wrong exactly where it matters —
	// a container's is a random hash, which would land on the board as-is. Asking
	// with it as the default costs one keystroke and lets it be corrected.
	machine, err := promptFn(labelMachine+" ["+defaultMachine+"]", false)
	if err != nil {
		return setup{}, err
	}
	if machine == "" {
		machine = defaultMachine
	}
	if server == "" || token == "" {
		return setup{}, errors.New("the server URL and the token are both required")
	}
	return setup{server: server, token: token, machine: machine}, nil
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
