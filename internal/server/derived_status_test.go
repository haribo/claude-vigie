package server

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/store"
)

// A session's status is not always the one the store holds: when reports stop, a
// live session is shown `stale` on an unwatched machine and `ended` on a watched
// one (#285). Everything derived from it must follow the status the client is
// actually shown — a session displayed `stale` that carried `working`'s rank
// would sit among the active ones for as long as it existed, which is the sort
// silently lying rather than failing.
//
// This is a guard rather than a regression test: the defect never shipped. It was
// caught while writing #617, by the derivation colliding with the package it was
// meant to call.
func TestDerivedFieldsFollowTheEffectiveStatus(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	long := now.Add(-time.Hour).UTC().Format(time.RFC3339)

	cases := []struct {
		name       string
		watched    bool
		wantStatus string
	}{
		{"an unwatched machine has gone quiet, not away", false, "stale"},
		{"a watched machine's silence means the session ended", true, "ended"},
	}
	for _, c := range cases {
		// Stored as working, last reported an hour ago.
		s := store.Session{ID: "s", Machine: "m", Status: "working", ReportedAt: long}
		v := toView(s, nil, now, c.watched)

		if v.Status != c.wantStatus {
			t.Fatalf("%s: status = %q, want %q", c.name, v.Status, c.wantStatus)
		}
		if want := status.Rank(c.wantStatus); v.Rank != want {
			t.Errorf("%s: rank = %d, want %d — the rank of %q, not of the stored %q",
				c.name, v.Rank, want, c.wantStatus, s.Status)
		}
		if want := status.NeedsAttention(c.wantStatus); v.Attention != want {
			t.Errorf("%s: attention = %v, want %v", c.name, v.Attention, want)
		}
	}
}
