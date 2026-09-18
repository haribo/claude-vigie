//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// session is a real `claude` running on a pseudo-terminal, with the master side
// kept here so keystrokes can be sent and the screen read back.
type session struct {
	cmd    *exec.Cmd
	master *os.File
	screen *os.File // everything the session painted, for reading afterwards
}

// startSession launches `claude` on a fresh pty in dir, writing the raw screen to
// screenPath.
//
// **Two things make a child session invisible, and both are load-bearing.**
//
// `CLAUDE_CODE_CHILD_SESSION` is inherited by any Claude Code launched from inside
// a Claude Code session, and it turns transcript saving *and* the registry record
// off. Left in place, the session runs, answers, does its work — and vigie cannot
// see it at all, because neither of the two files it reads is written. The first
// run of this harness measured nothing for that reason and looked like a bug in
// the harness.
//
// The others go with it: a session that believes it is a child also declines to
// own the terminal in ways that are not worth enumerating. Clearing all four is
// what makes this a session like the operator's.
func startSession(dir, screenPath string, extraArgs []string) (*session, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening /dev/ptmx: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("unlocking the pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("naming the pty: %w", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("opening the pty slave: %w", err)
	}
	defer func() { _ = slave.Close() }()

	// A terminal Claude Code is willing to draw on. Without a size it renders as if
	// the window were zero-wide and the prompt never becomes typeable.
	_ = unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Row: 50, Col: 160})

	screen, err := os.Create(screenPath) //nolint:gosec // a path this tool was given
	if err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("creating the screen log: %w", err)
	}

	cmd := exec.Command("claude", append([]string{"--permission-mode", "bypassPermissions"}, extraArgs...)...) //nolint:gosec // fixed binary, caller-chosen flags
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = childEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = screen.Close()
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	s := &session{cmd: cmd, master: master, screen: screen}
	go s.drain()
	return s, nil
}

// inherited names the variables that mark a Claude Code session as a child of
// another one. See startSession for why clearing them is the difference between
// measuring something and measuring nothing.
var inherited = []string{
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDECODE",
	"CLAUDE_CODE_SSE_PORT",
	"CLAUDE_CODE_ENTRYPOINT",
}

func childEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !hasAnyPrefix(kv, inherited) {
			out = append(out, kv)
		}
	}
	return append(out, "TERM=xterm-256color")
}

func hasAnyPrefix(kv string, names []string) bool {
	for _, n := range names {
		if len(kv) > len(n) && kv[:len(n)] == n && kv[len(n)] == '=' {
			return true
		}
	}
	return false
}

// drain copies the screen to the log. A pty master that nobody reads fills its
// buffer and blocks the session, so this runs for the session's whole life.
func (s *session) drain() {
	buf := make([]byte, 64*1024)
	for {
		n, err := s.master.Read(buf)
		if n > 0 {
			_, _ = s.screen.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// send types keys into the session, exactly as a keyboard would.
func (s *session) send(keys string) error {
	_, err := s.master.WriteString(keys)
	return err
}

func (s *session) close() {
	_ = s.send("\x03") // interrupt whatever is running
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}
	_ = s.master.Close()
	_ = s.screen.Close()
}
