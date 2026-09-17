package main

import (
	"testing"
	"time"
)

// #843. The tool's job is to make a claim re-runnable, so the parts that could
// quietly mis-state it are the ones worth pinning: the share it prints, and the
// line it draws between a residual that is certainly stale and one the transcript
// cannot speak for.

// A rate is printed as "one in N" because that is the form both documents state it
// in. Rounding it the wrong way, or dividing the wrong way round, would flatter or
// damn the residual without anyone noticing.
func TestARateIsStatedTheWayTheDocumentsStateIt(t *testing.T) {
	for _, c := range []struct {
		n, total int
		want     string
	}{
		{19, 866, "one in 46"},
		{8, 21, "one in 3"},
		{1, 1, "one in 1"},
		{0, 866, "none"}, // no residual is not "one in infinity"
		{5, 0, "none"},   // nothing launched: a share of nothing is not a number
	} {
		if got := rate(c.n, c.total); got != c.want {
			t.Errorf("rate(%d, %d) = %q, want %q", c.n, c.total, got, c.want)
		}
	}
}

// The split is a reporting device, never a rule about a session — ADR-0015 refuses
// a clock deciding what vigie cannot observe, and nothing here feeds a status.
// What it must not do is call a residual stale on a transcript that names no
// instant: "cannot say" is the honest answer, and the safe direction.
func TestAnUnreadableInstantIsNeverCalledStale(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for _, bad := range []string{"", "not a time", "2026-09-17"} {
		if silent(bad, now) {
			t.Errorf("silent(%q) said the session is certainly over; the transcript names no instant", bad)
		}
	}
	if !silent("2026-09-17T09:00:00Z", now) {
		t.Error("three hours of silence was not counted as certainly over")
	}
	if silent("2026-09-17T11:30:00Z", now) {
		t.Error("thirty minutes of silence was called certainly over — the work may still be running")
	}
}
