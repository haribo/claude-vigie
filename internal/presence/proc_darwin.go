package presence

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// macOS has no /proc: the process table is read through sysctl (ADR-0018).

// inspect reads one process: its parent and start time from kern.proc.pid, its
// name from argv[0]. p_comm is no use as a name — Claude Code's executable is its
// version file, so the kernel calls it "2.1.296" and only argv[0] says "claude".
func inspect(pid int) (process, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return process{}, fmt.Errorf("sysctl kern.proc.pid %d: %w", pid, err)
	}
	if int(k.Proc.P_pid) != pid {
		return process{}, fmt.Errorf("sysctl kern.proc.pid %d: %w", pid, errNoProcess)
	}
	p := process{ppid: int(k.Eproc.Ppid), start: uint64(k.Proc.P_starttime.Sec)} //nolint:gosec // a start time is never negative
	if argv, err := procArgs(pid); err == nil && len(argv) > 0 {
		p.name = filepath.Base(argv[0])
	}
	return p, nil
}

// errNoProcess marks a pid the kernel has no record for.
var errNoProcess = errors.New("no such process")

// absent reports whether err means the kernel has no such process. For a pid
// with no record, kern.proc.pid succeeds with zero bytes, and x/sys reports the
// size mismatch as EIO — so EIO is the "absent" answer here, and anything else
// (a refused read) is not evidence (#663).
func absent(err error) bool {
	return errors.Is(err, unix.EIO) || errors.Is(err, unix.ESRCH) || errors.Is(err, errNoProcess)
}

// ParseProcStart reads the `procStart` Claude Code writes into its session
// registry. On macOS it is the start time as a C-style date in UTC ("Mon Oct 12
// 09:56:18 2026"), to the second — the same instant kern.proc.pid reports, so
// both sides become seconds since the epoch. It returns 0 — "no pid-reuse
// guard" — for anything else.
func ParseProcStart(raw string) uint64 {
	t, err := time.Parse(time.ANSIC, raw)
	if err != nil || t.Unix() <= 0 {
		return 0
	}
	return uint64(t.Unix())
}

// WatcherRunning reports whether another process on this machine is running
// binName's `watch` subcommand (#371): a process other than selfPID whose
// argv[0] has binName as its base and whose arguments contain a bare "watch".
// Processes whose arguments the kernel will not show — other users' — are not
// ours to find, and are skipped.
func WatcherRunning(binName string, selfPID int) (bool, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return false, fmt.Errorf("sysctl kern.proc.all: %w", err)
	}
	for _, k := range procs {
		pid := int(k.Proc.P_pid)
		if pid <= 0 || pid == selfPID {
			continue
		}
		argv, err := procArgs(pid)
		if err != nil || len(argv) == 0 || filepath.Base(argv[0]) != binName {
			continue // vanished mid-scan, not readable, or a different binary
		}
		for _, a := range argv[1:] {
			if a == "watch" {
				return true, nil
			}
		}
	}
	return false, nil
}

// procArgs returns a process's argv from kern.procargs2, whose layout is: argc
// (int32, host order), the executable path, NUL padding, then argc
// NUL-terminated arguments, then the environment.
func procArgs(pid int) ([]string, error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	if len(buf) < 4 {
		return nil, fmt.Errorf("procargs2 for pid %d: %d bytes", pid, len(buf))
	}
	argc := int(binary.NativeEndian.Uint32(buf[:4]))
	rest := buf[4:]
	// Skip the executable path, then its padding.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil, fmt.Errorf("procargs2 for pid %d: no executable path", pid)
	}
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	argv := make([]string, 0, argc)
	for len(argv) < argc && len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			j = len(rest)
		}
		argv = append(argv, string(rest[:j]))
		rest = rest[min(j+1, len(rest)):]
	}
	return argv, nil
}
