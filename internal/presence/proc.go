package presence

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// maxDepth bounds the ancestor walk, so a broken process chain cannot loop.
const maxDepth = 20

// process is what the ancestor walk and the liveness check need to know about
// one pid. How each field is read is per platform (ADR-0006 on Linux, ADR-0018
// on macOS); what they mean is not.
type process struct {
	name  string // what Claude Code is recognized by: comm on Linux, argv[0]'s base on macOS
	ppid  int
	start uint64 // a pid-reuse guard; its unit is the platform's, compared only on the same machine
}

// claudeName is the name the Claude Code process carries on every platform.
const claudeName = "claude"

// ResolveClaude walks the ancestor chain from the current process up to the
// nearest "claude" process and returns its mapping. It is meant to be called
// from a hook (a descendant of claude); it errors if no claude ancestor is
// found (e.g. when not run under Claude Code, or on an unsupported platform).
func ResolveClaude() (Mapping, error) {
	pid := os.Getpid()
	for i := 0; i < maxDepth; i++ {
		p, err := inspect(pid)
		if err != nil {
			return Mapping{}, err
		}
		if p.name == claudeName {
			return Mapping{PID: pid, StartTime: p.start}, nil
		}
		if p.ppid <= 1 {
			break
		}
		pid = p.ppid
	}
	return Mapping{}, errors.New("no claude ancestor process found")
}

// Liveness is what the process table can say about a mapped process. The third
// value is the point: reading it can fail for reasons that say nothing about the
// process — a hardened `hidepid`, a container or namespace that does not expose
// the pid — and treating those as death declared a whole fleet ended at the next
// scan (#663).
//
// ADR-0006 promises a fallback where presence is unavailable, and it holds for
// the path that goes through capture: nothing is written, so nothing is read.
// `registryDead` takes its pid straight from Claude Code's registry and never
// touches capture, so it needed the distinction to exist here.
type Liveness int

const (
	// Unknown: the process table could not be read. It is not evidence of anything.
	Unknown Liveness = iota
	// Live: present, with the start time the mapping recorded.
	Live
	// Gone: the process table says there is no such process — or the pid was
	// reused, which means the mapped one is gone just the same.
	Gone
)

// Status reports what the process table can say about the mapped process,
// distinguishing a process that is absent from one that could not be looked at.
func Status(m Mapping) Liveness {
	p, err := inspect(m.PID)
	switch {
	case err == nil && p.start == m.StartTime:
		return Live
	case err == nil:
		return Gone // the pid is in use by a younger process: ours ended
	case absent(err):
		return Gone
	default:
		return Unknown // permissions, a namespace, an unparsable record, an unsupported platform
	}
}

// Alive reports whether the mapped process is still the same live process.
// Callers that must not mistake "could not look" for "not there" ask Status.
func Alive(m Mapping) bool { return Status(m) == Live }

// procRoot is where the Linux process table is read from. It is a variable for
// one reason: the interesting case of `Status` is a /proc that *cannot be read*,
// and a test cannot arrange that on the real one. Nothing outside tests assigns it.
var procRoot = "/proc"

// readStat parses /proc/<pid>/stat, returning the process comm (field 2), its
// parent pid (field 4), and its start time (field 22). comm is parenthesized
// and may itself contain spaces and parentheses, so the numeric fields are read
// only after the final ')'.
//
// It is the Linux reader, but it is only file parsing and builds everywhere, so
// the tests that exercise it against a fake /proc compile on every platform.
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
