package client

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Reading the token with no echo at all is what sudo and ssh do, but it leaves a
// newcomer unsure their keystrokes register. Echoing one asterisk per character
// fixes that and costs nothing in secrecy: `vigied token` always emits 32 random
// bytes hex-encoded, so a token is *always* 64 characters — the asterisk count
// reveals a constant of the system, not anything about this token.
//
// The price is doing the line editing by hand, because raw mode turns off the
// terminal's own. That is what this file is: a pure editor, so the rules are
// tested without a terminal, plus the smallest possible amount of raw-mode I/O
// around it (#407).

// errSecretCanceled reports a Ctrl-C at the prompt. Raw mode disables the signal,
// so the interrupt arrives as a byte and must be handled rather than delivered.
var errSecretCanceled = errors.New("canceled")

type secretAction int

const (
	secretContinue secretAction = iota
	secretDone
	secretCancel
)

// secretEditor accumulates a typed secret. It is a struct rather than a function
// because an escape sequence spans several bytes: an arrow key sends ESC, '[' and
// a letter, and without that state the '[' and the letter land in the token.
type secretEditor struct {
	buf []byte
	esc escState
}

// escState tracks where we are in an escape sequence. Two states are needed, not
// one: after ESC the next byte may be the CSI introducer ('[' or 'O'), and '[' is
// itself inside the 0x40–0x7E range that ends a CSI — so treating it as a
// terminator would let the sequence's real final byte through as typed input.
type escState int

const (
	escNone escState = iota
	escSeen          // an ESC was read; the next byte says what kind
	escCSI           // inside a CSI sequence, consuming until its final byte
)

// key folds one input byte in and returns what to echo. The echo is written by
// the caller, so this stays pure and testable.
func (e *secretEditor) key(b byte) (echo string, action secretAction) {
	switch e.esc {
	case escSeen:
		if b == '[' || b == 'O' {
			e.esc = escCSI // a CSI or SS3 sequence: keep consuming
		} else {
			e.esc = escNone // a two-byte sequence, already complete
		}
		return "", secretContinue
	case escCSI:
		if b >= 0x40 && b <= 0x7e { // the final byte of a CSI sequence
			e.esc = escNone
		}
		return "", secretContinue
	case escNone:
	}

	switch b {
	case '\r', '\n':
		return "\r\n", secretDone
	case 0x03: // Ctrl-C
		return "\r\n", secretCancel
	case 0x15: // Ctrl-U — discard the line
		n := len(e.buf)
		e.buf = e.buf[:0]
		return strings.Repeat("\b \b", n), secretContinue
	case 0x7f, 0x08: // backspace / delete
		if len(e.buf) == 0 {
			return "", secretContinue
		}
		e.buf = e.buf[:len(e.buf)-1]
		return "\b \b", secretContinue
	case 0x1b: // ESC — an escape sequence starts
		e.esc = escSeen
		return "", secretContinue
	}

	if b < 0x20 {
		return "", secretContinue // any other control byte is ignored
	}
	e.buf = append(e.buf, b)
	return "*", secretContinue
}

// Terminal seams. readMasked is the one place that touches raw mode, and raw mode
// cannot be entered on a pipe — without these the loop below is untestable and
// the guarantee that matters most (the terminal is always restored) rests on
// nothing but review.
var (
	makeRaw      = term.MakeRaw
	restoreTerm  = term.Restore
	readPassword = term.ReadPassword
)

// readMasked reads a secret from f, echoing asterisks to w. The terminal is put
// in raw mode and **always** restored — leaving it raw would give the operator a
// shell with no echo, which is a far worse outcome than any prompt.
//
// When raw mode is unavailable (not a terminal, or unsupported) it falls back to
// an echo-less read rather than echoing the secret in cooked mode.
func readMasked(f *os.File, w io.Writer) (string, error) {
	state, err := makeRaw(f.Fd())
	if err != nil {
		b, perr := readPassword(f.Fd())
		return string(b), perr
	}
	defer func() { _ = restoreTerm(f.Fd(), state) }()

	var ed secretEditor
	one := make([]byte, 1)
	for {
		n, rerr := f.Read(one)
		if n > 0 {
			echo, action := ed.key(one[0])
			if echo != "" {
				_, _ = io.WriteString(w, echo)
			}
			switch action {
			case secretDone:
				return string(ed.buf), nil
			case secretCancel:
				return "", errSecretCanceled
			case secretContinue:
			}
		}
		if rerr != nil {
			if rerr == io.EOF && len(ed.buf) > 0 {
				return string(ed.buf), nil
			}
			return "", rerr
		}
	}
}
