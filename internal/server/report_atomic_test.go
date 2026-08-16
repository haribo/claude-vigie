package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// #512. handleReport read a session, merged the report in Go, and wrote it back.
// Nothing held the row in between. SQLite serializes writes, not a
// read-modify-write cycle, so two reports for the same session interleave: the
// second write is computed from a snapshot the first has already replaced.
//
// Every reconciliation rule in that file is "given the current status and its
// source, decide". Those rules are correct, and they were reading a value that
// could already be stale — so the outcome was decided by commit order rather
// than by the rules. That is #190 / #201 / #233 back, as a race instead of a
// logic bug.
//
// `TestConcurrentWritesDoNotBusyError` writes distinct session ids, so it
// exercises the write lock and never the lost update. Two writers on one id is a
// different test, and this is it.

// The scenario the rules already have an answer for: a hook says the operator is
// the blocker, the watcher says the transcript is quiet. `waiting` must win,
// whatever the order the two land in.
func TestAHookAndAWatchOnTheSameSessionDoNotLoseEachOther(t *testing.T) {
	const rounds = 60
	for i := 0; i < rounds; i++ {
		srv := newTestServer(t)

		post := func(r api.ReportRequest) {
			r.SessionID, r.Machine = "s", "m"
			body, _ := json.Marshal(r)
			_ = do(t, srv, http.MethodPost, "/api/report", body, true)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			post(api.ReportRequest{Event: "Notification", NotificationType: "permission_prompt"})
		}()
		go func() {
			defer wg.Done()
			post(api.ReportRequest{
				Event: "watch", Status: "idle",
				WatcherVersion: version.Version, WatcherCommit: version.Commit,
			})
		}()
		wg.Wait()

		rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		var views []api.SessionView
		if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
			t.Fatal(err)
		}
		if len(views) != 1 {
			t.Fatalf("round %d: %d sessions, want 1", i, len(views))
		}
		// The watcher's blind `idle` never clears a hook-set `waiting` — that rule
		// is tested in isolation and holds. It can only be lost to a stale read.
		if got := views[0].Status; got != "waiting" {
			t.Fatalf("round %d: status = %q, want waiting — a stale snapshot overwrote the hook's report", i, got)
		}
	}
}

// The other half of the lost update: accumulated usage. Two reports each adding
// tokens must both survive, or a read-modify-write drops one silently — and
// tokens are the figure nobody can spot as wrong by looking.
func TestConcurrentReportsDoNotLoseAccumulatedUsage(t *testing.T) {
	const writers = 8
	srv := newTestServer(t)

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(api.ReportRequest{
				SessionID: "s", Machine: "m", Event: "Stop",
				Usage: &api.Usage{OutputTokens: 100},
			})
			_ = do(t, srv, http.MethodPost, "/api/report", body, true)
		}()
	}
	wg.Wait()

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("%d sessions, want 1", len(views))
	}
	// A report carries the session's cumulative total, not a delta, so the last
	// writer's figure is the answer — what must not happen is a *lower* one, which
	// is what a stale snapshot writes.
	if got := views[0].Usage.OutputTokens; got != 100 {
		t.Errorf("output tokens = %d, want 100", got)
	}
}
