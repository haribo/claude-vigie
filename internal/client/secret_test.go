package client

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/term"
)

// drive feeds a byte sequence through the editor and returns what was typed, what
// was echoed, and how it ended.
func drive(in string) (typed, echo string, action secretAction) {
	var ed secretEditor
	var b strings.Builder
	for i := 0; i < len(in); i++ {
		e, a := ed.key(in[i])
		b.WriteString(e)
		if a != secretContinue {
			return string(ed.buf), b.String(), a
		}
	}
	return string(ed.buf), b.String(), secretContinue
}

// TestSecretEditorEchoesOneStarPerCharacter is the whole point: the operator must
// see that their keystrokes register, without the secret appearing.
func TestSecretEditorEchoesOneStarPerCharacter(t *testing.T) {
	typed, echo, action := drive("abc123\r")
	if typed != "abc123" {
		t.Errorf("typed = %q", typed)
	}
	if echo != "******\r\n" {
		t.Errorf("echo = %q, want six stars then a newline", echo)
	}
	if action != secretDone {
		t.Errorf("action = %v, want done", action)
	}
	if strings.ContainsAny(echo, "abc123") {
		t.Error("the secret leaked into the echo")
	}
}

func TestSecretEditorBackspace(t *testing.T) {
	typed, echo, _ := drive("ab\x7f\x7fc\r") // type ab, erase both, type c
	if typed != "c" {
		t.Errorf("typed = %q, want c", typed)
	}
	// two stars, two erases, one star, newline
	if echo != "**"+"\b \b"+"\b \b"+"*"+"\r\n" {
		t.Errorf("echo = %q", echo)
	}
	// Backspace on an empty buffer must not erase the prompt itself.
	if _, echo, _ := drive("\x7f\x7f\r"); echo != "\r\n" {
		t.Errorf("backspace on empty echoed %q, want nothing", echo)
	}
}

func TestSecretEditorCtrlU(t *testing.T) {
	typed, echo, _ := drive("secret\x15new\r")
	if typed != "new" {
		t.Errorf("typed = %q, want new", typed)
	}
	if !strings.Contains(echo, strings.Repeat("\b \b", 6)) {
		t.Errorf("Ctrl-U should erase the six stars, echo = %q", echo)
	}
}

// TestSecretEditorCtrlCCancels: raw mode disables the signal, so Ctrl-C arrives
// as a byte. Failing to handle it would make the prompt impossible to escape.
func TestSecretEditorCtrlCCancels(t *testing.T) {
	typed, _, action := drive("abc\x03")
	if action != secretCancel {
		t.Fatalf("action = %v, want cancel", action)
	}
	if typed != "abc" {
		t.Errorf("typed = %q", typed) // the caller discards it; it must not be returned
	}
}

// TestSecretEditorIgnoresEscapeSequences is the reason the editor holds state: an
// arrow key sends ESC '[' 'A', and without swallowing the sequence the '[' and
// the 'A' would land in the token.
func TestSecretEditorIgnoresEscapeSequences(t *testing.T) {
	typed, echo, _ := drive("ab\x1b[Ac\r") // type ab, press up, type c
	if typed != "abc" {
		t.Errorf("typed = %q, want abc — the escape sequence leaked", typed)
	}
	if echo != "***\r\n" {
		t.Errorf("echo = %q, want three stars", echo)
	}
	// A multi-parameter sequence (e.g. a mouse or Home key) is swallowed whole.
	if typed, _, _ := drive("a\x1b[1;5Db\r"); typed != "ab" {
		t.Errorf("typed = %q, want ab", typed)
	}
}

func TestSecretEditorIgnoresOtherControlBytes(t *testing.T) {
	typed, _, _ := drive("a\tb\x00c\r")
	if typed != "abc" {
		t.Errorf("typed = %q, want abc", typed)
	}
}

// TestSecretEditorAcceptsPunctuationAndUTF8: a token is hex today, but a pasted
// value must survive intact — the editor appends raw bytes, so multi-byte runes
// simply echo more than one star.
func TestSecretEditorAcceptsPastedBytes(t *testing.T) {
	typed, _, _ := drive("aB3-_=+/é\r")
	if typed != "aB3-_=+/é" {
		t.Errorf("typed = %q", typed)
	}
}

// fakeTerminal stubs raw mode so the read loop can run on a pipe, and records
// whether the terminal was handed back.
type fakeTerminal struct {
	rawFailed bool
	restored  bool
	password  string
	passErr   error
}

func (ft *fakeTerminal) install(t *testing.T) {
	t.Helper()
	oR, oS, oP := makeRaw, restoreTerm, readPassword
	makeRaw = func(uintptr) (*term.State, error) {
		if ft.rawFailed {
			return nil, errors.New("not a terminal")
		}
		return &term.State{}, nil
	}
	restoreTerm = func(uintptr, *term.State) error { ft.restored = true; return nil }
	readPassword = func(uintptr) ([]byte, error) { return []byte(ft.password), ft.passErr }
	t.Cleanup(func() { makeRaw, restoreTerm, readPassword = oR, oS, oP })
}

// pipeWith returns a readable file preloaded with in, closed so the loop sees EOF.
func pipeWith(t *testing.T, in string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	go func() { _, _ = w.WriteString(in); _ = w.Close() }()
	return r
}

// TestReadMaskedEchoesAndRestores covers the loop that the pure editor cannot:
// the secret comes back whole, only asterisks are written, and — the guarantee
// that matters most — the terminal is handed back.
func TestReadMaskedEchoesAndRestores(t *testing.T) {
	ft := &fakeTerminal{}
	ft.install(t)
	var echo strings.Builder

	got, err := readMasked(pipeWith(t, "s3cr3t\r"), &echo)
	if err != nil {
		t.Fatalf("readMasked: %v", err)
	}
	if got != "s3cr3t" {
		t.Errorf("secret = %q", got)
	}
	if echo.String() != "******\r\n" {
		t.Errorf("echo = %q, want six stars", echo.String())
	}
	if strings.Contains(echo.String(), "s3cr3t") {
		t.Error("the secret leaked into the echo")
	}
	if !ft.restored {
		t.Error("the terminal was left in raw mode")
	}
}

// TestReadMaskedRestoresOnCancel: leaving a terminal raw would give the operator
// a shell with no echo, so restoring must not depend on the happy path.
func TestReadMaskedRestoresOnCancel(t *testing.T) {
	ft := &fakeTerminal{}
	ft.install(t)
	var echo strings.Builder

	if _, err := readMasked(pipeWith(t, "abc\x03"), &echo); !errors.Is(err, errSecretCanceled) {
		t.Errorf("err = %v, want canceled", err)
	}
	if !ft.restored {
		t.Error("the terminal was left in raw mode after Ctrl-C")
	}
}

// TestReadMaskedFallsBackWithoutRawMode: with no terminal, it must fall back to an
// echo-less read rather than echoing the secret in cooked mode.
func TestReadMaskedFallsBackWithoutRawMode(t *testing.T) {
	ft := &fakeTerminal{rawFailed: true, password: "from-fallback"}
	ft.install(t)
	var echo strings.Builder

	got, err := readMasked(pipeWith(t, "ignored\r"), &echo)
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-fallback" {
		t.Errorf("secret = %q, want the fallback read", got)
	}
	if echo.String() != "" {
		t.Errorf("the fallback echoed %q; a secret must never be echoed", echo.String())
	}
	if ft.restored {
		t.Error("restored a terminal that was never put in raw mode")
	}
}

// TestReadMaskedAcceptsInputEndingWithoutNewline: a pasted value whose stream ends
// at EOF must not be lost.
func TestReadMaskedAcceptsInputEndingWithoutNewline(t *testing.T) {
	ft := &fakeTerminal{}
	ft.install(t)
	var echo strings.Builder

	got, err := readMasked(pipeWith(t, "no-newline"), &echo)
	if err != nil {
		t.Fatalf("readMasked: %v", err)
	}
	if got != "no-newline" {
		t.Errorf("secret = %q", got)
	}
	if !ft.restored {
		t.Error("the terminal was left in raw mode")
	}
}
