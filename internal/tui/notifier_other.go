//go:build !darwin

package tui

import (
	"os"
	"os/exec"
)

// notifyBinary is the freedesktop notifier (libnotify).
const notifyBinary = "notify-send"

// notifyCommand builds the notification. Fixed argv, no shell: title and body are
// notify-send's display text, not a command, so there is no injection surface.
func notifyCommand(title, body string) *exec.Cmd {
	//nolint:gosec // constant command, arguments are display text only
	return exec.Command(notifyBinary, "-u", "normal", "-a", "vigie", title, body)
}

// graphicalSession reports whether this process can reach an X11 or Wayland display.
//
//nolint:forbidigo // DISPLAY/WAYLAND_DISPLAY are a platform probe, not config
func graphicalSession() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}
