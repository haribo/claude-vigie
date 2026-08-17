package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

// These are #432, reproduced from a production figure: one (day, model) row held
// 61 051 295 773 output tokens where the session itself reported 2 713 408 — the
// whole total re-added on nearly every scan for half a day. `stats_daily` is
// never pruned and never recomputed, so a wrong value is permanent.

type rollupFixture struct {
	srv *Server
	st  *store.Store
	t   *testing.T
}

func newRollupFixture(t *testing.T) *rollupFixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "rollup.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &rollupFixture{srv: New(st, testToken, nil), st: st, t: t}
}

// report sends one watch report carrying the session's cumulative output total,
// exactly as the watcher does.
func (f *rollupFixture) report(sessionID string, total int64) {
	f.t.Helper()
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
		SessionID: sessionID, Machine: "m", Status: "working", Model: "claude-opus-4-8",
		Usage:     &api.Usage{OutputTokens: total},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if rec := do(f.t, f.srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
		f.t.Fatalf("report(%s, %d) = %d", sessionID, total, rec.Code)
	}
}

func (f *rollupFixture) counted() int64 {
	f.t.Helper()
	rows, err := f.st.ListDailyStats(context.Background(), "")
	if err != nil {
		f.t.Fatal(err)
	}
	var n int64
	for _, r := range rows {
		n += r.OutputTokens
	}
	return n
}

// TestRollupIgnoresARegressingTotal is the mechanism behind the production
// figure: one session written to two transcript files, so each scan reported the
// real total and then zero. The drop is skipped as a delta but still overwrites
// what was stored, so the next rise reads as brand-new output.
func TestRollupIgnoresARegressingTotal(t *testing.T) {
	f := newRollupFixture(t)

	for i := 0; i < 5; i++ {
		f.report("s1", 2_713_408) // the live transcript
		f.report("s1", 0)         // the abandoned one, same session id
	}

	if got := f.counted(); got != 2_713_408 {
		t.Errorf("counted %d, want 2713408 — the total was re-counted %.1f times",
			got, float64(got)/2_713_408)
	}
}

// TestRollupCountsGrowthOnceAfterARegression: after a drop, only genuinely new
// output counts. Returning to a figure already counted must add nothing.
func TestRollupCountsGrowthOnceAfterARegression(t *testing.T) {
	f := newRollupFixture(t)

	f.report("s1", 1_000)
	f.report("s1", 0)     // regression
	f.report("s1", 1_000) // back to where it was: nothing new
	if got := f.counted(); got != 1_000 {
		t.Fatalf("counted %d after returning to a known figure, want 1000", got)
	}

	f.report("s1", 1_500) // 500 genuinely new
	if got := f.counted(); got != 1_500 {
		t.Errorf("counted %d, want 1500 — growth past the previous peak must count exactly once", got)
	}
}

// TestRollupSurvivesRetention is the second reproduction, and the reason the mark
// cannot live on the session row: a session quiet for longer than --max-age stops
// being reported, --session-retention deletes its row, and resuming it re-adds a
// whole lifetime of tokens to today.
func TestRollupSurvivesRetention(t *testing.T) {
	f := newRollupFixture(t)
	ctx := context.Background()

	f.report("s1", 1_000_000)
	if got := f.counted(); got != 1_000_000 {
		t.Fatalf("counted %d before the prune, want 1000000", got)
	}

	n, err := f.st.PruneSessions(ctx, 0, time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d sessions, want 1 — the setup did not reproduce the case", n)
	}

	f.report("s1", 1_000_000) // resumed; the transcript never changed

	if got := f.counted(); got != 1_000_000 {
		t.Errorf("counted %d after resuming a pruned session, want 1000000", got)
	}
}

// TestRollupKeepsSessionsIndependent: the mark is per session, so one session's
// history cannot mask another's output.
func TestRollupKeepsSessionsIndependent(t *testing.T) {
	f := newRollupFixture(t)

	f.report("big", 1_000_000)
	f.report("small", 5_000)
	f.report("small", 6_000)

	if got := f.counted(); got != 1_006_000 {
		t.Errorf("counted %d, want 1006000", got)
	}
}

// TestRollupStillCountsOrdinaryGrowth guards the fix from over-correcting: normal
// monotonic growth must accumulate exactly, with no loss.
func TestRollupStillCountsOrdinaryGrowth(t *testing.T) {
	f := newRollupFixture(t)

	for _, total := range []int64{100, 250, 250, 900, 1_400} {
		f.report("s1", total)
	}
	if got := f.counted(); got != 1_400 {
		t.Errorf("counted %d, want 1400", got)
	}
}
