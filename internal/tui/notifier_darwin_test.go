package tui

import (
	"strings"
	"testing"
)

// The macOS notifier (ADR-0018, #869). A session's name is chosen by whoever runs the
// session, so it must reach AppleScript as data: spliced into the script source,
// a name ending in `" & (do shell script "…")` would run as code.
func TestNotifyCommandPassesTextAsArgv(t *testing.T) {
	title := `vigie — x" & (do shell script "touch /tmp/owned") & "`
	body := `is "waiting" \ for input`
	cmd := notifyCommand(title, body)

	if cmd.Args[0] != "osascript" {
		t.Fatalf("notifier = %q, want osascript", cmd.Args[0])
	}
	n := len(cmd.Args)
	if cmd.Args[n-2] != title || cmd.Args[n-1] != body {
		t.Errorf("title/body are not the trailing argv: %q", cmd.Args[n-2:])
	}
	// Everything before them is the fixed script, which never contains the text.
	for _, a := range cmd.Args[1 : n-2] {
		if strings.Contains(a, "do shell script") || strings.Contains(a, "waiting") {
			t.Errorf("display text leaked into the script: %q", a)
		}
	}
	if strings.Join(cmd.Args[1:n-2], " ") != strings.Join(notifyScript, " ") {
		t.Errorf("script = %q, want the fixed one", cmd.Args[1:n-2])
	}
}

func TestGraphicalSessionIsRuledOutOverSSH(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	if !graphicalSession() {
		t.Error("a local login read as headless")
	}
	t.Setenv("SSH_CONNECTION", "192.0.2.1 50000 192.0.2.2 22")
	if graphicalSession() {
		t.Error("an SSH login read as a graphical session")
	}
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "/dev/ttys004")
	if graphicalSession() {
		t.Error("an SSH tty read as a graphical session")
	}
}
