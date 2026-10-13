//go:build !linux && !darwin

package presence

import (
	"errors"
	"strconv"
)

// errUnsupported is what every process lookup answers where vigie has no reader.
// It is deliberately not "absent": a platform that cannot look must never read as
// one that looked and found nothing (#663).
var errUnsupported = errors.New("process inspection is not supported on this platform")

func inspect(int) (process, error) { return process{}, errUnsupported }

func absent(error) bool { return false }

// ParseProcStart keeps the Linux reading: without a reader, the value is only
// carried, never compared.
func ParseProcStart(raw string) uint64 {
	ps, _ := strconv.ParseUint(raw, 10, 64)
	return ps
}

// WatcherRunning cannot scan here; the error makes callers fall back.
func WatcherRunning(string, int) (bool, error) { return false, errUnsupported }
