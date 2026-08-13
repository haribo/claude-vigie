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

// TestAskSetupAsksForEverything: init takes no flags and reads no environment, so
// the three values come from the three questions — and the token is asked for as a
// secret, since echoing it would merely move it from the shell history to the
// terminal scrollback.
func TestAskSetupAsksForEverything(t *testing.T) {
	p := &stubPrompt{answers: map[string]string{
		labelServer:              "http://typed:8080",
		labelToken:               "typed-token",
		labelMachine + " [host]": "typed-box",
	}}
	withPrompt(t, true, p)

	got, err := askSetup("host")
	if err != nil {
		t.Fatal(err)
	}
	if got.server != "http://typed:8080" || got.token != "typed-token" || got.machine != "typed-box" {
		t.Errorf("got %+v", got)
	}
	if len(p.asked) != 3 {
		t.Fatalf("asked %v, want three questions", p.asked)
	}
	if p.secrets[0] || !p.secrets[1] || p.secrets[2] {
		t.Errorf("only the token may be read without echo, got %v", p.secrets)
	}
}

// TestAskSetupKeepsTheHostnameOnEnter: the default exists so the usual answer is
// one keystroke.
func TestAskSetupKeepsTheHostnameOnEnter(t *testing.T) {
	p := &stubPrompt{answers: map[string]string{
		labelServer: "http://typed:8080",
		labelToken:  "typed-token",
	}} // the machine answer is empty: the operator just pressed enter
	withPrompt(t, true, p)

	got, err := askSetup("my-host")
	if err != nil {
		t.Fatal(err)
	}
	if got.machine != "my-host" {
		t.Errorf("machine = %q, want the hostname default", got.machine)
	}
}

// TestAskSetupNeedsATerminal keeps a scripted run from blocking forever on a
// question nobody can answer.
func TestAskSetupNeedsATerminal(t *testing.T) {
	p := &stubPrompt{}
	withPrompt(t, false, p)

	if _, err := askSetup("host"); !errors.Is(err, errNoTerminal) {
		t.Errorf("err = %v, want errNoTerminal", err)
	}
	if len(p.asked) != 0 {
		t.Errorf("asked %v with no terminal", p.asked)
	}
}

// TestAskSetupRejectsEmptyAnswers: pressing enter through the prompts must not
// write a config that cannot work.
func TestAskSetupRejectsEmptyAnswers(t *testing.T) {
	p := &stubPrompt{answers: map[string]string{}}
	withPrompt(t, true, p)

	if _, err := askSetup("host"); err == nil {
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
