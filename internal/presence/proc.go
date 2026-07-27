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

// Alive reports whether the mapped process is still the same live process:
// present in /proc and with an unchanged start time (guarding against pid reuse).
func Alive(m Mapping) bool {
	_, _, start, err := readStat(m.PID)
	if err != nil {
		return false
	}
	return start == m.StartTime
}

// readStat parses /proc/<pid>/stat, returning the process comm (field 2), its
// parent pid (field 4), and its start time (field 22). comm is parenthesized
// and may itself contain spaces and parentheses, so the numeric fields are read
// only after the final ')'.
func readStat(pid int) (comm string, ppid int, starttime uint64, err error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
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
