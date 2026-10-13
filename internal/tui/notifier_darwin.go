package tui

import (
	"os"
	"os/exec"
)

// notifyBinary is macOS's own scripting host: present on every install, unlike
// notify-send or terminal-notifier (ADR-0018).
const notifyBinary = "osascript"

// notifyScript reads title and body from its argv, so neither is ever spliced
// into AppleScript source: a session name with quotes or a `do shell script` in
// it stays display text.
var notifyScript = []string{
	"-e", "on run argv",
	"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
	"-e", "end run",
}

func notifyCommand(title, body string) *exec.Cmd {
	//nolint:gosec // constant script; title and body are passed as data in argv
	return exec.Command(notifyBinary, append(append([]string{}, notifyScript...), title, body)...)
}

// graphicalSession reports whether a notification would reach the operator's
// screen. macOS sets no DISPLAY; what rules a notification out is a login over
// SSH, which is not the session the Notification Center shows.
//
//nolint:forbidigo // SSH_CONNECTION/SSH_TTY are a platform probe, not config
func graphicalSession() bool {
	return os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == ""
}
