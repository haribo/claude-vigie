package client

import (
	"errors"
	"io"
	"os"
	"testing"
)

// stubPrompt records what was asked and answers from a script.
type stubPrompt struct {
	asked   []string
	secrets []bool
	answers map[string]string
}

func (s *stubPrompt) ask(label string, secret bool) (string, error) {
	s.asked = append(s.asked, label)
	s.secrets = append(s.secrets, secret)
	return s.answers[label], nil
}

func withPrompt(t *testing.T, tty bool, p *stubPrompt) {
	t.Helper()
	origTTY, origAsk := stdinIsTerminal, promptFn
	stdinIsTerminal = func() bool { return tty }
	promptFn = p.ask
	t.Cleanup(func() { stdinIsTerminal, promptFn = origTTY, origAsk })
}

// TestResolveEndpointPrefersFlags: an explicit flag must never be overridden by
// the environment, and must never trigger a question.
func TestResolveEndpointPrefersFlags(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "http://from-env:8080")
	t.Setenv("VIGIE_TOKEN", "env-token")
	p := &stubPrompt{}
	withPrompt(t, true, p)

	srv, tok, err := resolveEndpoint("http://from-flag:8080", "flag-token")
	if err != nil {
		t.Fatal(err)
	}
	if srv != "http://from-flag:8080" || tok != "flag-token" {
		t.Errorf("got %q/%q, want the flag values", srv, tok)
	}
	if len(p.asked) != 0 {
		t.Errorf("asked %v with both flags given", p.asked)
	}
}

// TestResolveEndpointFallsBackToEnv covers the automation path: containers and
// provisioning supply the values without putting the token on a command line.
func TestResolveEndpointFallsBackToEnv(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "http://from-env:8080")
	t.Setenv("VIGIE_TOKEN", "env-token")
	p := &stubPrompt{}
	withPrompt(t, false, p) // no terminal: this must still work

	srv, tok, err := resolveEndpoint("", "")
	if err != nil {
		t.Fatal(err)
	}
	if srv != "http://from-env:8080" || tok != "env-token" {
		t.Errorf("got %q/%q, want the environment values", srv, tok)
	}
	if len(p.asked) != 0 {
		t.Errorf("asked %v without a terminal", p.asked)
	}
}

// TestResolveEndpointNonInteractiveFails is the one that keeps a provisioning run
// from hanging forever on a prompt nobody can answer.
func TestResolveEndpointNonInteractiveFails(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "")
	t.Setenv("VIGIE_TOKEN", "")
	p := &stubPrompt{}
	withPrompt(t, false, p)

	if _, _, err := resolveEndpoint("", ""); !errors.Is(err, errNoEndpoint) {
		t.Errorf("err = %v, want errNoEndpoint", err)
	}
	if len(p.asked) != 0 {
		t.Errorf("asked %v with no terminal", p.asked)
	}
}

// TestResolveEndpointAsksForWhatIsMissing: only the missing value is asked for,
// and the token is read as a secret — echoing it would merely move the token from
// the shell history to the terminal scrollback.
func TestResolveEndpointAsksForWhatIsMissing(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "")
	t.Setenv("VIGIE_TOKEN", "")
	p := &stubPrompt{answers: map[string]string{labelToken: "typed-token"}}
	withPrompt(t, true, p)

	srv, tok, err := resolveEndpoint("http://from-flag:8080", "")
	if err != nil {
		t.Fatal(err)
	}
	if srv != "http://from-flag:8080" || tok != "typed-token" {
		t.Errorf("got %q/%q", srv, tok)
	}
	if len(p.asked) != 1 || p.asked[0] != labelToken {
		t.Fatalf("asked %v, want only the token", p.asked)
	}
	if !p.secrets[0] {
		t.Error("the token was asked for with echo on")
	}
}

// TestResolveEndpointAsksForBoth: nothing supplied, but somebody is there.
func TestResolveEndpointAsksForBoth(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "")
	t.Setenv("VIGIE_TOKEN", "")
	p := &stubPrompt{answers: map[string]string{
		labelServer: "http://typed:8080",
		labelToken:  "typed-token",
	}}
	withPrompt(t, true, p)

	srv, tok, err := resolveEndpoint("", "")
	if err != nil {
		t.Fatal(err)
	}
	if srv != "http://typed:8080" || tok != "typed-token" {
		t.Errorf("got %q/%q", srv, tok)
	}
	if len(p.asked) != 2 {
		t.Fatalf("asked %v, want both", p.asked)
	}
	if p.secrets[0] {
		t.Error("the server URL is not a secret and should echo")
	}
	if !p.secrets[1] {
		t.Error("the token was asked for with echo on")
	}
}

// TestResolveEndpointRejectsEmptyAnswers: pressing enter twice must not write a
// config that cannot work.
func TestResolveEndpointRejectsEmptyAnswers(t *testing.T) {
	t.Setenv("VIGIE_SERVER", "")
	t.Setenv("VIGIE_TOKEN", "")
	p := &stubPrompt{answers: map[string]string{}} // every answer empty
	withPrompt(t, true, p)

	if _, _, err := resolveEndpoint("", ""); err == nil {
		t.Error("empty answers should not produce a config")
	}
}

// TestReadLineDoesNotOverRead is the regression for a defect the first
// implementation shipped with: reading the prompt through a bufio.Reader
// swallowed the *following* line, so term.ReadPassword then waited forever for a
// token that had already been consumed. A human typing one line at a time never
// hits it; a pipe or a fast paste always does.
func TestReadLineDoesNotOverRead(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err := w.WriteString("http://example:8080\nthe-secret-token\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	first, err := readLine(r)
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if first != "http://example:8080" {
		t.Errorf("first line = %q", first)
	}
	// The second line must still be there for the password reader.
	rest, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "the-secret-token\n" {
		t.Errorf("the next line was consumed: remaining = %q", rest)
	}
}
