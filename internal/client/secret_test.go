package client

import (
	"strings"
	"testing"
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
