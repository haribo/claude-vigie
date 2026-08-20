package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// countingStore is the real store with one method watched. Embedding the
// interface rather than reimplementing eighteen methods keeps the fake honest:
// everything except the call being counted is the production behavior.
type countingStore struct {
	Store
	recentSamples int
}

func (c *countingStore) RecentSamples(ctx context.Context, since string, limit int) (map[string][]int64, error) {
	c.recentSamples++
	return c.Store.RecentSamples(ctx, since, limit)
}

// TestListingSessionsCostsOneSampleReadWhateverTheFleetSize guards #580.
//
// The route used to read one session's samples per session, on the path the TUI,
// the browser and the GNOME indicator all refetch whenever anything on the board
// changes. The assertion is the number of reads, not the wall clock: a timing
// bound is what a loaded runner makes flaky, while "one read" is the shape the
// fix is about and cannot pass by accident.
func TestListingSessionsCostsOneSampleReadWhateverTheFleetSize(t *testing.T) {
	for _, sessions := range []int{1, 25} {
		st, err := store.Open(t.TempDir() + "/test.db")
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })

		counting := &countingStore{Store: st}
		srv := New(counting, testToken, nil)
		ctx := context.Background()
		for i := range sessions {
			id := string(rune('a'+i%26)) + string(rune('a'+i/26))
			if err := st.UpsertSession(ctx, store.Session{ID: id, Machine: "orion", Status: "working"}); err != nil {
				t.Fatal(err)
			}
		}

		rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("%d sessions: status %d", sessions, rec.Code)
		}
		var got []api.SessionView
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%d sessions: decoding: %v", sessions, err)
		}
		if len(got) != sessions {
			t.Fatalf("rendered %d sessions, want %d", len(got), sessions)
		}
		if counting.recentSamples != 1 {
			t.Errorf("%d sessions cost %d sample reads, want 1 — the read must not grow with the fleet",
				sessions, counting.recentSamples)
		}
	}
}
