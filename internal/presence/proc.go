package presence

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// maxDepth bounds the ancestor walk, so a broken /proc chain cannot loop.
const maxDepth = 20

// procRoot is where the process table is read from. It is a variable for one
// reason: the interesting case of `Status` is a /proc that *cannot be read*, and
// a test cannot arrange that on the real one. Nothing outside tests assigns it.
var procRoot = "/proc"

// ResolveClaude walks the ancestor chain from the current process up to the
// nearest "claude" process and returns its mapping. It is meant to be called
// from a hook (a descendant of claude); it errors if no claude ancestor is
// found (e.g. when not run under Claude Code, or off Linux).
func ResolveClaude() (Mapping, error) {
	pid := os.Getpid()
	for i := 0; i < maxDepth; i++ {
		comm, ppid, start, err := readStat(pid)
		if err != nil {
			return Mapping{}, err
		}
		if comm == "claude" {
			return Mapping{PID: pid, StartTime: start}, nil
		}
		if ppid <= 1 {
			break
		}
		pid = ppid
	}
	return Mapping{}, errors.New("no claude ancestor process found")
}

// Liveness is what /proc can say about a mapped process. The third value is the
// point: reading /proc can fail for reasons that say nothing about the process —
// a hardened `hidepid`, a container or namespace that does not expose the pid —
// and treating those as death declared a whole fleet ended at the next scan
// (#663).
//
// ADR-0006 promises a fallback where presence is unavailable, and it holds for
// the path that goes through capture: nothing is written, so nothing is read.
// `registryDead` takes its pid straight from Claude Code's registry and never
// touches capture, so it needed the distinction to exist here.
type Liveness int

const (
	// Unknown: /proc could not be read. It is not evidence of anything.
	Unknown Liveness = iota
	// Live: present, with the start time the mapping recorded.
	Live
	// Gone: /proc says there is no such process — or the pid was reused, which
	// means the mapped one is gone just the same.
	Gone
)

// Status reports what /proc can say about the mapped process, distinguishing a
// process that is absent from one that could not be looked at.
func Status(m Mapping) Liveness {
	_, _, start, err := readStat(m.PID)
	switch {
	case err == nil && start == m.StartTime:
		return Live
	case err == nil:
		return Gone // the pid is in use by a younger process: ours ended
	case errors.Is(err, os.ErrNotExist):
		return Gone
	default:
		return Unknown // permissions, a namespace, an unparsable file
	}
}

// Alive reports whether the mapped process is still the same live process.
// Callers that must not mistake "could not look" for "not there" ask Status.
func Alive(m Mapping) bool { return Status(m) == Live }

// commMax is the kernel's TASK_COMM_LEN-1: /proc/<pid>/stat's comm is truncated
// to 15 bytes, so a binary name is matched against its first 15 bytes.
const commMax = 15

// WatcherRunning reports whether another process on this machine is running
// binName's `watch` subcommand — a local liveness signal independent of the
// server heartbeat (#371). It scans /proc for a process (other than selfPID)
// whose comm matches binName (truncated to the kernel's 15-byte limit) and whose
// argv contains a bare "watch" token. Linux only: it returns an error if /proc
// cannot be read (e.g. off Linux), so callers can fall back rather than treat an
// unreadable /proc as "not running".
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

// readStat parses /proc/<pid>/stat, returning the process comm (field 2), its
// parent pid (field 4), and its start time (field 22). comm is parenthesized
// and may itself contain spaces and parentheses, so the numeric fields are read
// only after the final ')'.
func readStat(pid int) (comm string, ppid int, starttime uint64, err error) {
	data, err := os.ReadFile(fmt.Sprintf("%s/%d/stat", procRoot, pid))
	if err != nil {
		return "", 0, 0, err
	}
	s := string(data)
	open := strings.IndexByte(s, '(')
	closeParen := strings.LastIndexByte(s, ')')
	if open < 0 || closeParen < 0 || closeParen < open {
		return "", 0, 0, fmt.Errorf("malformed stat for pid %d", pid)
	}
	comm = s[open+1 : closeParen]

	// After ')', fields are: [0]=state [1]=ppid ... [19]=starttime (field 22).
	fields := strings.Fields(s[closeParen+1:])
	if len(fields) < 20 {
		return "", 0, 0, fmt.Errorf("too few stat fields for pid %d", pid)
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("parsing ppid for pid %d: %w", pid, err)
	}
	starttime, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return "", 0, 0, fmt.Errorf("parsing starttime for pid %d: %w", pid, err)
	}
	return comm, ppid, starttime, nil
}
