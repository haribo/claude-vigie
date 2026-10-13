package presence

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// inspect reads one process from /proc (ADR-0006).
func inspect(pid int) (process, error) {
	comm, ppid, start, err := readStat(pid)
	if err != nil {
		return process{}, err
	}
	return process{name: comm, ppid: ppid, start: start}, nil
}

// absent reports whether err means /proc has no such process, as opposed to a
// /proc that refused to answer (#663).
func absent(err error) bool { return errors.Is(err, os.ErrNotExist) }

// ParseProcStart reads the `procStart` Claude Code writes into its session
// registry: on Linux, /proc's start time in clock ticks, as a decimal string.
// It returns 0 — "no pid-reuse guard" — for anything else.
func ParseProcStart(raw string) uint64 {
	ps, _ := strconv.ParseUint(raw, 10, 64)
	return ps
}

// WatcherRunning reports whether another process on this machine is running
// binName's `watch` subcommand — a local liveness signal independent of the
// server heartbeat (#371). It scans /proc for a process (other than selfPID)
// whose comm matches binName (truncated to the kernel's 15-byte limit) and whose
// argv contains a bare "watch" token. It returns an error if /proc cannot be
// read, so callers can fall back rather than treat an unreadable /proc as "not
// running".
func WatcherRunning(binName string, selfPID int) (bool, error) {
	want := binName
	if len(want) > commMax {
		want = want[:commMax]
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, fmt.Errorf("reading /proc: %w", err)
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == selfPID {
			continue // not a pid dir, or ourselves
		}
		comm, _, _, err := readStat(pid)
		if err != nil || comm != want {
			continue // process vanished mid-scan, or a different binary
		}
		if cmdlineHasArg(pid, "watch") {
			return true, nil
		}
	}
	return false, nil
}

// commMax is the kernel's TASK_COMM_LEN-1: /proc/<pid>/stat's comm is truncated
// to 15 bytes, so a binary name is matched against its first 15 bytes.
const commMax = 15

// cmdlineHasArg reports whether /proc/<pid>/cmdline contains arg as a whole
// NUL-separated token. A read failure (the process may exit mid-scan) is false,
// not an error — the caller only needs a positive match.
func cmdlineHasArg(pid int, arg string) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	for _, tok := range strings.Split(string(data), "\x00") {
		if tok == arg {
			return true
		}
	}
	return false
}
