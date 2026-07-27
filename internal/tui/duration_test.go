package tui

import (
	"testing"
	"time"
)

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{3 * time.Second, "3s"},
		{59 * time.Second, "59s"},
		{12 * time.Minute, "12m"},
		{90 * time.Minute, "1h"},
		{25 * time.Hour, "1d"},
	}
	for _, c := range cases {
		if got := humanizeDuration(c.d); got != c.want {
			t.Errorf("humanizeDuration(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestRelativeAge(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	if got := relativeAge(now.Add(-90*time.Second).Format(time.RFC3339), now); got != "1m" {
		t.Errorf("relativeAge(90s ago) = %q, want 1m", got)
	}
	if got := relativeAge("", now); got != "-" {
		t.Errorf("relativeAge(empty) = %q, want -", got)
	}
	if got := relativeAge("not-a-time", now); got != "-" {
		t.Errorf("relativeAge(bad) = %q, want -", got)
	}
	// A future timestamp clamps to 0.
	if got := relativeAge(now.Add(time.Hour).Format(time.RFC3339), now); got != "0s" {
		t.Errorf("relativeAge(future) = %q, want 0s", got)
	}
}
