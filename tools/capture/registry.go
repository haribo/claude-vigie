package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// entry is one registry record, reduced to the five fields the watcher actually
// reads (`internal/watch/registry.go`). Everything else Claude Code writes there
// — the conversation name, the working directory, the bridge id — is never parsed,
// so it cannot be written by accident.
type entry struct {
	SessionID  string
	Status     string
	WaitingFor string
	PID        int
	ProcStart  uint64
}

// readRegistry mirrors the watcher's own read, best-effort by the same rule: a
// file that will not parse is skipped rather than failing the recording.
func readRegistry(dir string) map[string]entry {
	m := map[string]entry{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // confined to the registry dir
		if err != nil {
			continue
		}
		var raw struct {
			SessionID  string `json:"sessionId"`
			Status     string `json:"status"`
			WaitingFor string `json:"waitingFor"`
			PID        int    `json:"pid"`
			ProcStart  string `json:"procStart"`
		}
		if err := json.Unmarshal(data, &raw); err != nil || raw.SessionID == "" {
			continue
		}
		ps, _ := strconv.ParseUint(raw.ProcStart, 10, 64)
		m[raw.SessionID] = entry{
			SessionID: raw.SessionID, Status: raw.Status, WaitingFor: raw.WaitingFor,
			PID: raw.PID, ProcStart: ps,
		}
	}
	return m
}

// tailBytes bounds how much of a transcript is read to find its last dated line.
// A transcript grows to megabytes and the answer is always at the end; reading the
// whole file on every change would make the recorder the slowest thing on the
// machine it is supposed to observe unnoticed.
const tailBytes = 128 << 10

// transcriptAge returns how long ago the session's transcript last carried a
// dated line, and whether one was found.
//
// **An age, never an instant.** The watcher turns this into a report timestamp and
// the reconciliation compares it against when a status was posted, so the
// *interval* is the whole of what a fixture needs. An absolute time would add one
// thing the interval does not carry: when the operator works.
func transcriptAge(projects, sessionID string, now time.Time) (time.Duration, bool) {
	matches, err := filepath.Glob(filepath.Join(projects, "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return 0, false
	}
	ts, ok := lastDatedLine(matches[0])
	if !ok {
		return 0, false
	}
	return now.Sub(ts), true
}

// lastDatedLine reads the tail of a transcript and returns the timestamp of the
// last line carrying one. Only the timestamp is extracted — no line is retained,
// and no other field is even looked at.
func lastDatedLine(path string) (time.Time, bool) {
	f, err := os.Open(path) //nolint:gosec // a path this tool was pointed at
	if err != nil {
		return time.Time{}, false
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return time.Time{}, false
	}
	size := fi.Size()
	start := size - tailBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return time.Time{}, false
	}
	return lastTimestampIn(buf, start > 0)
}

// lastTimestampIn scans buffered JSONL for the last `"timestamp":"…"` value.
// dropFirst discards the leading fragment when the buffer began mid-line.
func lastTimestampIn(buf []byte, dropFirst bool) (time.Time, bool) {
	lines := strings.Split(string(buf), "\n")
	if dropFirst && len(lines) > 0 {
		lines = lines[1:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var o struct {
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal([]byte(lines[i]), &o) != nil || o.Timestamp == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, o.Timestamp); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
