package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

// #508. A session blocked on a permission prompt is reported `waiting` by the
// Notification hook — the one status only a hook can see. The prompt freezes the
// transcript on an unanswered tool_use, which is exactly the shape the watcher
// reads as `stalled`, and about 45 s later it says so.
//
// Both guards missed it. `holdsWaiting` listed working/thinking/compacting;
// `reconcileWatch` defended `waiting` against `idle` only. `stalled` fell through
// to "a positive change is the watcher's" and took the status.
//
// It does not go quiet, it lies: both statuses call the operator, so the
// attention queue looks the same size while naming the wrong cause — and the
// operator goes looking at a hung tool instead of answering the prompt that is
// waiting for them. `stalled` is watch-owned, so the hook's `waiting` does not
// come back.
//
// The defense was written for #235, before `stalled` existed (#256). Nothing
// widened it when it did, which is the argument for defending against the
// *class* rather than a hand-kept list.

// waitingFixture posts a Notification (→ waiting) and returns a reporter and a
// status reader for that session, on an injected clock.
func waitingFixture(t *testing.T) (report func(api.ReportRequest), status func() string, advance func(time.Duration)) {
	t.Helper()
	srv := newTestServer(t)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	srv.clock = func() time.Time { return now }

	report = func(r api.ReportRequest) {
		t.Helper()
		r.SessionID, r.Machine = "blocked", "m"
		if r.Timestamp == "" {
			r.Timestamp = now.UTC().Format(time.RFC3339)
		}
		body, _ := json.Marshal(r)
		if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
			t.Fatalf("report %+v = %d", r, rec.Code)
		}
	}
	status = func() string {
		t.Helper()
		rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		var views []api.SessionView
		if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			if v.ID == "blocked" {
				return v.Status
			}
		}
		t.Fatal("session not found")
		return ""
	}
	advance = func(d time.Duration) { now = now.Add(d) }

	report(api.ReportRequest{Event: "Notification", NotificationType: "permission_prompt"})
	if got := status(); got != "waiting" {
		t.Fatalf("setup: status = %q, want waiting", got)
	}
	return report, status, advance
}

// watchReport is a watcher scan carrying an inferred status, timestamped at the
// transcript's mtime — which a permission prompt has frozen before the prompt
// was posted.
func watchReport(status, timestamp string) api.ReportRequest {
	return api.ReportRequest{
		Event: "watch", Status: status, Timestamp: timestamp,
		WatcherVersion: version.Version, WatcherCommit: version.Commit,
	}
}

// The reported failure, at the width it happens: the transcript froze when the
// prompt appeared, so the watcher's report predates the waiting it would replace.
func TestAPermissionWaitingSurvivesAnInferredStalled(t *testing.T) {
	report, status, advance := waitingFixture(t)
	frozen := time.Date(2026, 8, 16, 11, 59, 30, 0, time.UTC).Format(time.RFC3339)

	advance(50 * time.Second) // past stalledAfter
	report(watchReport("stalled", frozen))

	if got := status(); got != "waiting" {
		t.Errorf("status = %q, want waiting — the operator is the blocker, not a hung tool", got)
	}
}

// The same for every status the watcher can only *infer* while the transcript is
// frozen. A hand-kept list is what failed here, so the guard is asserted against
// the whole class rather than the one status that was reported.
func TestAPermissionWaitingSurvivesEveryInferredStatus(t *testing.T) {
	frozen := time.Date(2026, 8, 16, 11, 59, 30, 0, time.UTC).Format(time.RFC3339)
	for _, inferred := range []string{"working", "thinking", "compacting", "stalled", "idle"} {
		t.Run(inferred, func(t *testing.T) {
			report, status, advance := waitingFixture(t)
			advance(50 * time.Second)
			report(watchReport(inferred, frozen))
			if got := status(); got != "waiting" {
				t.Errorf("an inferred %q replaced the hook's waiting (got %q)", inferred, got)
			}
		})
	}
}

// The guard must not become a latch. What the watcher *positively observes* still
// wins, and a transcript that moves past the prompt releases the session — that is
// the rule #235 established and this must not widen into "waiting is permanent".
func TestWaitingStillYieldsToObservationAndToMovement(t *testing.T) {
	frozen := time.Date(2026, 8, 16, 11, 59, 30, 0, time.UTC).Format(time.RFC3339)

	// A dead process is an observation, not an inference: it wins even frozen.
	report, status, _ := waitingFixture(t)
	report(watchReport("ended", frozen))
	if got := status(); got != "ended" {
		t.Errorf("a positively observed `ended` was refused: got %q", got)
	}

	// An API error is observed in the transcript, not inferred from silence.
	report, status, _ = waitingFixture(t)
	report(watchReport("error", frozen))
	if got := status(); got != "error" {
		t.Errorf("a positively observed `error` was refused: got %q", got)
	}

	// And the transcript moving past the prompt releases the turn (#235).
	report, status, advance := waitingFixture(t)
	advance(2 * time.Minute)
	report(watchReport("working", time.Date(2026, 8, 16, 12, 1, 0, 0, time.UTC).Format(time.RFC3339)))
	if got := status(); got != "working" {
		t.Errorf("the transcript moved past the prompt and the session stayed %q, want working", got)
	}
}

// #636. `timeAfter` used to be documented as "false on any parse error, so a
// missing timestamp never holds anything", while `holdsWaiting` negates it — so
// an absent timestamp holds the waiting rather than releasing it.
//
// That is the right way round: this rule exists to demand evidence that the
// transcript moved past when waiting was posted, and a report with no timestamp
// carries none. The comment said the opposite, which is the kind of sentence
// someone corrects the code to match. This pins the behavior so they cannot.
func TestAWatchReportWithNoTimestampCannotClearAWaiting(t *testing.T) {
	sess := store.Session{Status: "waiting", StatusSource: "hook", StatusChangedAt: "2026-08-28T12:00:00Z"}
	inferred := func(ts string) api.ReportRequest {
		return api.ReportRequest{Event: "watch", Status: "idle", Timestamp: ts}
	}
	for _, c := range []struct {
		why  string
		ts   string
		want bool
	}{
		{"no timestamp is no evidence the transcript moved", "", true},
		{"nor is one that will not parse", "not-a-date", true},
		{"one older than the waiting is evidence it did not", "2026-08-28T11:00:00Z", true},
		{"one newer than the waiting is the evidence this rule asks for", "2026-08-28T12:00:01Z", false},
	} {
		if got := holdsWaiting(sess, inferred(c.ts)); got != c.want {
			t.Errorf("holdsWaiting(ts=%q) = %v, want %v — %s", c.ts, got, c.want, c.why)
		}
	}
}
