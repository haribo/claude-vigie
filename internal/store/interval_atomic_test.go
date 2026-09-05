package store

import (
	"context"
	"testing"
)

// #737. The event log is the mark the next status interval is measured from, so
// "already counted" and "recorded" have to become true together. They did not:
// the seconds were added first and the append was best-effort, so a failed append
// left the mark on the old event and the next hook remeasured a span whose first
// half was already counted — permanently, since stats_daily is never recomputed.
//
// Both directions are induced bluntly, the way #669's guard is: any error on
// either write must leave the other alone.

// A trigger that refuses the `Stop` event, so the append fails while the read
// that precedes it still works — the shape the defect actually took. The event
// name is written into the trigger because SQLite refuses a bound parameter
// there.
func refuseStopEvents(t *testing.T, st *Store) {
	t.Helper()
	if _, err := st.db.Exec(
		`CREATE TRIGGER refuse BEFORE INSERT ON events WHEN NEW.event = 'Stop'
		 BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatal(err)
	}
}

func seedInterval(t *testing.T, st *Store) context.Context {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertSession(ctx, sampleSession("s1")); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvent(ctx, Event{
		SessionID: "s1", Event: "UserPromptSubmit", Status: "working",
		CreatedAt: "2026-09-05T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func workingSeconds(t *testing.T, st *Store) int64 {
	t.Helper()
	stats, err := st.ListDailyStats(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range stats {
		total += r.WorkingSeconds
	}
	return total
}

func closeAt(ctx context.Context, st *Store, event, at string) error {
	return st.AppendEventClosingInterval(ctx,
		Event{SessionID: "s1", Event: event, Status: "working", CreatedAt: at},
		func(last Event, ok bool) StatusInterval {
			if !ok {
				return StatusInterval{}
			}
			return StatusInterval{Day: "2026-09-05", Model: "opus", Status: last.Status,
				Secs: 60}
		})
}

func TestNoSecondsLandWhenTheEventCannotBeRecorded(t *testing.T) {
	st := openTestStore(t)
	ctx := seedInterval(t, st)
	refuseStopEvents(t, st)

	if err := closeAt(ctx, st, "Stop", "2026-09-05T10:01:00Z"); err == nil {
		t.Fatal("no error from a close whose event cannot be appended")
	}
	if got := workingSeconds(t, st); got != 0 {
		t.Errorf("stats_daily gained %d seconds while the mark stayed on the old event; "+
			"the next hook will count that span again and the day is inflated for good", got)
	}
}

func TestTheMarkDoesNotMoveWhenTheSecondsCannotLand(t *testing.T) {
	st := openTestStore(t)
	ctx := seedInterval(t, st)
	if _, err := st.db.ExecContext(ctx, `DROP TABLE stats_daily`); err != nil {
		t.Fatal(err)
	}

	if err := closeAt(ctx, st, "Stop", "2026-09-05T10:01:00Z"); err == nil {
		t.Fatal("no error from a close whose daily write cannot land")
	}
	last, ok, err := st.LastEvent(ctx, "s1")
	if err != nil || !ok {
		t.Fatalf("LastEvent = ok %v, err %v", ok, err)
	}
	if last.CreatedAt != "2026-09-05T10:00:00Z" {
		t.Errorf("the mark advanced to %s over seconds that were never counted", last.CreatedAt)
	}
}

// The working path: each interval counted once, and the mark following along.
func TestEachIntervalIsCountedOnce(t *testing.T) {
	st := openTestStore(t)
	ctx := seedInterval(t, st)

	if err := closeAt(ctx, st, "Stop", "2026-09-05T10:01:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := closeAt(ctx, st, "UserPromptSubmit", "2026-09-05T10:02:00Z"); err != nil {
		t.Fatal(err)
	}
	if got := workingSeconds(t, st); got != 120 {
		t.Errorf("working_seconds = %d, want 120 — two one-minute intervals, once each", got)
	}
	last, _, err := st.LastEvent(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if last.CreatedAt != "2026-09-05T10:02:00Z" {
		t.Errorf("mark = %s, want the last event", last.CreatedAt)
	}
}
