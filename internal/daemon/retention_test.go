package daemon

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/server"
	"github.com/haribo/claude-vigie/internal/store"
)

// The retention decision deletes session rows. Every branch of it used to be
// uncovered (#442), and its consequences are not local: deleting a row while the
// session's transcript lives on is the mechanism that made the token rollup
// re-count a whole lifetime total (#432).
//
// The branch that matters most is the one that decides *not* to prune. An
// inverted test there would delete data on a fleet that had switched retention
// off, silently and irreversibly.

func TestDecideRetention(t *testing.T) {
	const def = 24 * time.Hour

	for _, tc := range []struct {
		name      string
		stored    string
		storedOK  bool
		want      time.Duration
		wantPrune bool
		warns     bool
	}{
		{name: "nothing stored yet uses the default", storedOK: false, want: def, wantPrune: true},
		{name: "a stored window overrides the default", stored: "72h", storedOK: true, want: 72 * time.Hour, wantPrune: true},
		{name: "an empty stored value disables pruning", stored: "", storedOK: true, wantPrune: false},
		{name: "a zero window disables pruning", stored: "0s", storedOK: true, wantPrune: false},
		{name: "a negative window disables pruning", stored: "-1h", storedOK: true, wantPrune: false},
		{name: "an unparsable value falls back to the default, and says so", stored: "forever", storedOK: true, want: def, wantPrune: true, warns: true},
		{name: "a stored window shorter than an hour is honored", stored: "5m", storedOK: true, want: 5 * time.Minute, wantPrune: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decideRetention(tc.stored, tc.storedOK, def)
			if got.prune != tc.wantPrune {
				t.Fatalf("prune = %v, want %v", got.prune, tc.wantPrune)
			}
			if got.prune && got.window != tc.want {
				t.Errorf("window = %v, want %v", got.window, tc.want)
			}
			if warned := got.warning != ""; warned != tc.warns {
				t.Errorf("warning = %q, wanted one: %v", got.warning, tc.warns)
			}
		})
	}
}

// A default of zero means the operator disabled retention on the command line;
// nothing stored must not resurrect it.
func TestDecideRetentionWithADisabledDefault(t *testing.T) {
	for _, stored := range []struct {
		v  string
		ok bool
	}{{"", false}, {"", true}, {"nonsense", true}} {
		if got := decideRetention(stored.v, stored.ok, 0); got.prune {
			t.Errorf("stored=%q ok=%v: pruning enabled with a zero default", stored.v, stored.ok)
		}
	}
}

// The unparsable case must name the value, or an operator cannot tell which
// setting is being ignored.
func TestUnparsableRetentionWarningNamesTheValue(t *testing.T) {
	got := decideRetention("forever", true, 24*time.Hour)
	if !strings.Contains(got.warning, "forever") {
		t.Errorf("warning = %q, want it to quote the offending value", got.warning)
	}
	if !strings.Contains(got.warning, "24h") {
		t.Errorf("warning = %q, want it to say what is used instead", got.warning)
	}
}

// TestPruneLoopSeedsTheRetentionSetting covers the first-run side effect: the
// default is written into meta so the settings API has something to show.
func TestPruneLoopSeedsTheRetentionSetting(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	if _, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); ok {
		t.Fatal("the setting exists before anything ran")
	}

	// pruneLoop blocks on a ticker, so run it in the background and observe the
	// seeding it performs before the first tick.
	go pruneLoop(st, 36*time.Hour, testLogger())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); ok {
			if v != "36h0m0s" {
				t.Fatalf("seeded %q, want the default retention", v)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("the retention setting was never seeded")
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestSeedRetentionLeavesKeepAllAlone is the #656 regression. Settings offers
// `off (keep all)`, which stores an empty retention — the operator saying keep
// every session. The seed used to treat "stored, but empty" the same as "never
// stored" and write the default over it on the next daemon start, so a deploy or
// a `Restart=on-failure` silently deleted every session quiet for 24 h.
//
// `GetMeta` already separates the two (absent → ok=false, empty → ok=true), and
// `decideRetention` honors the distinction; only the seed erased it.
func TestSeedRetentionLeavesKeepAllAlone(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	// What the TUI writes for `off (keep all)` and the API stores verbatim.
	if err := st.SetMeta(ctx, server.RetentionMetaKey, ""); err != nil {
		t.Fatal(err)
	}

	seedRetention(ctx, st, 24*time.Hour)

	v, ok, err := st.GetMeta(ctx, server.RetentionMetaKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || v != "" {
		t.Errorf("keep-all became %q (present=%v) after a restart; the operator's sessions would be pruned", v, ok)
	}
}

// The first-run seed still happens: an absent key is not a choice.
func TestSeedRetentionWritesTheDefaultOnFirstRun(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	seedRetention(ctx, st, 36*time.Hour)

	if v, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); !ok || v != "36h0m0s" {
		t.Errorf("seeded %q (present=%v), want the default retention", v, ok)
	}
}
